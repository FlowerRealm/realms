#!/usr/bin/env bash
set -euo pipefail

# 统一 CI 入口（本地/CI 同口径）。
# - 默认不依赖真实上游 Secrets：Codex E2E 使用 fake upstream，Playwright 使用 seed 模式
# - 真实上游集成回归请使用: scripts/ci-real.sh（对应 GitHub Actions: ci-real.yml）

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

need_cmd go
need_cmd node
need_cmd npm
need_cmd codex

log "go test ./..."
go test ./...

log "codex e2e (fake upstream)"
export REALMS_CI_ENFORCE_E2E="1"
export REALMS_CI_MODEL="${REALMS_CI_MODEL:-gpt-5.2}"
go test ./tests/e2e -run TestCodexCLI_E2E_FakeUpstream_Cache -count=1

log "web e2e (playwright seed)"
npm --prefix web ci
if [[ "${CI:-}" != "" ]]; then
  (cd web && npx playwright install --with-deps chromium)
else
  (cd web && npx playwright install chromium)
fi
npm --prefix web run build
npm --prefix web run test:e2e:ci

log "OK"
