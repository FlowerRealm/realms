#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

log() {
  printf ">> %s\n" "$*"
}

need_cmd() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    echo "missing required command: ${name}" >&2
    exit 2
  fi
}

need_env() {
  local name="$1"
  if [[ "${!name:-}" == "" ]]; then
    echo "missing required env: ${name}" >&2
    exit 2
  fi
}

need_cmd go
need_cmd node
need_cmd npm
need_cmd codex
need_cmd curl

need_env REALMS_CI_UPSTREAM_BASE_URL
need_env REALMS_CI_UPSTREAM_API_KEY
need_env REALMS_CI_MODEL

retry() {
  local max_attempts="$1"
  local base_sleep_secs="$2"
  shift 2

  local attempt=1
  while true; do
    if "$@"; then
      return 0
    fi
    local rc="$?"
    if [[ "${attempt}" -ge "${max_attempts}" ]]; then
      return "${rc}"
    fi
    local sleep_secs="$(( base_sleep_secs * attempt ))"
    log "retry ${attempt}/${max_attempts} failed (rc=${rc}), sleeping ${sleep_secs}s: $*"
    sleep "${sleep_secs}"
    attempt="$(( attempt + 1 ))"
  done
}

log "go test ./..."
go test ./...

export REALMS_CI_ENFORCE_E2E="1"

log "codex e2e (concurrency regression, fake upstream SSE)"
go test ./tests/e2e -run TestCodexE2E_ConcurrentWindows_ProbeDueSSE -count=1

log "codex e2e (real upstream)"
retry 3 30 go test ./tests/e2e -run TestCodexCLI_E2E -count=1

if [[ "${REALMS_CI_CLI_RUNNER_URL:-}" != "" ]]; then
  log "cli channel test e2e (real upstream + real CLI runner)"
  retry 2 30 go test ./tests/e2e -run TestCLIChannelTest_RealUpstream_E2E -count=1 -timeout=120s
else
  log "skip: cli channel test e2e (REALMS_CI_CLI_RUNNER_URL not set)"
fi

log "web smoke (curl seed + real upstream)"
npm --prefix web ci
npm --prefix web run build

BASE_URL="${REALMS_E2E_BASE_URL:-http://127.0.0.1:18181}"
export REALMS_E2E_BASE_URL="${BASE_URL}"

existing_no_proxy="$(printf "%s" "${NO_PROXY:-${no_proxy:-}}" | xargs || true)"
no_proxy_parts=()
IFS=',' read -r -a no_proxy_parts <<<"${existing_no_proxy}"
append_no_proxy() {
  local host="$1"
  for part in "${no_proxy_parts[@]:-}"; do
    if [[ "$(printf "%s" "${part}" | xargs)" == "${host}" ]]; then
      return 0
    fi
  done
  no_proxy_parts+=("${host}")
}
append_no_proxy "127.0.0.1"
append_no_proxy "localhost"
append_no_proxy "::1"
merged_no_proxy="$(IFS=','; printf "%s" "${no_proxy_parts[*]// /}")"
export NO_PROXY="${merged_no_proxy}"
export no_proxy="${merged_no_proxy}"

mkdir -p "${ROOT_DIR}/output"
WEB_SMOKE_LOG="${ROOT_DIR}/output/ci-real-web-smoke.log"

web_smoke_on_err() {
  if [[ -f "${WEB_SMOKE_LOG}" ]]; then
    echo "web smoke: 失败，最近日志（${WEB_SMOKE_LOG}）:" >&2
    tail -n 200 "${WEB_SMOKE_LOG}" >&2 || true
  fi
}

cleanup_web_smoke() {
  if [[ "${WEB_SMOKE_PID:-}" != "" ]]; then
    kill "${WEB_SMOKE_PID}" >/dev/null 2>&1 || true
    wait "${WEB_SMOKE_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup_web_smoke EXIT
trap web_smoke_on_err ERR

export REALMS_E2E_ENFORCE_REAL_UPSTREAM="1"
export REALMS_E2E_UPSTREAM_BASE_URL="${REALMS_CI_UPSTREAM_BASE_URL}"
export REALMS_E2E_UPSTREAM_API_KEY="${REALMS_CI_UPSTREAM_API_KEY}"
export REALMS_E2E_BILLING_MODEL="${REALMS_CI_MODEL}"

addr="${REALMS_E2E_BASE_URL#http://}"
addr="${addr#https://}"
addr="${addr%%/*}"
export REALMS_E2E_ADDR="${addr:-127.0.0.1:18181}"
export REALMS_E2E_FRONTEND_DIST_DIR="${ROOT_DIR}/web/dist"

log "start: go run ./cmd/realms-e2e (${REALMS_E2E_BASE_URL})"
go run ./cmd/realms-e2e >"${WEB_SMOKE_LOG}" 2>&1 &
WEB_SMOKE_PID="$!"

wait_healthz() {
  local url="$1"
  local attempts="${2:-60}"
  local sleep_secs="${3:-1}"
  for _ in $(seq 1 "${attempts}"); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep "${sleep_secs}"
  done
  return 1
}

if ! wait_healthz "${REALMS_E2E_BASE_URL}/healthz" 90 1; then
  echo "web smoke: healthz 未就绪: ${REALMS_E2E_BASE_URL}/healthz" >&2
  tail -n 200 "${WEB_SMOKE_LOG}" >&2 || true
  exit 1
fi

healthz_json="$(curl -fsS "${REALMS_E2E_BASE_URL}/healthz")"
printf "%s" "${healthz_json}" | grep -q '"ok":true'
printf "%s" "${healthz_json}" | grep -q '"db_ok":true'

index_html="$(curl -fsS "${REALMS_E2E_BASE_URL}/")"
printf "%s" "${index_html}" | grep -q "Realms"
if printf "%s" "${index_html}" | grep -q "前端构建产物未发现"; then
  echo "web smoke: 仍在使用 fallback index（web/dist 未被正确加载）" >&2
  exit 1
fi

headers="$(curl -fsSI "${REALMS_E2E_BASE_URL}/assets/realms_icon.svg" | tr -d '\r')"
printf "%s" "${headers}" | grep -qi "^content-type: image/svg+xml"

log "OK"
