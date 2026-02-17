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

log "web e2e (seed + real upstream)"
npm --prefix web ci
if [[ "${CI:-}" != "" ]]; then
  (cd web && npx playwright install --with-deps chromium)
else
  (cd web && npx playwright install chromium)
fi
npm --prefix web run build
export REALMS_E2E_ENFORCE_REAL_UPSTREAM="1"
export REALMS_E2E_UPSTREAM_BASE_URL="${REALMS_CI_UPSTREAM_BASE_URL}"
export REALMS_E2E_UPSTREAM_API_KEY="${REALMS_CI_UPSTREAM_API_KEY}"
export REALMS_E2E_BILLING_MODEL="${REALMS_CI_MODEL}"
retry 2 60 npm --prefix web run test:e2e:ci

log "OK"
