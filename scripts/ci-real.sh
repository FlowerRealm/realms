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

log "go test ./..."
go test ./...

log "codex e2e (real upstream)"
export REALMS_CI_ENFORCE_E2E="1"
go test ./tests/e2e -run TestCodexE2E_ConcurrentWindows_ProbeDueSSE -count=1
go test ./tests/e2e -run TestCodexCLI_E2E -count=1

log "web e2e (seed + real upstream)"
npm --prefix web ci
if [[ "${CI:-}" != "" ]]; then
  (cd web && npx playwright install --with-deps chromium)
else
  (cd web && npx playwright install chromium)
fi
npm --prefix web run build
REALMS_E2E_ENFORCE_REAL_UPSTREAM=1 \
REALMS_E2E_UPSTREAM_BASE_URL="${REALMS_CI_UPSTREAM_BASE_URL}" \
REALMS_E2E_UPSTREAM_API_KEY="${REALMS_CI_UPSTREAM_API_KEY}" \
REALMS_E2E_BILLING_MODEL="${REALMS_CI_MODEL}" \
npm --prefix web run test:e2e:ci

log "OK"
