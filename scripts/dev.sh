#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

AIR_BIN="air"
if command -v air >/dev/null 2>&1; then
  AIR_BIN="$(command -v air)"
elif [[ -x "./.tmp/bin/air" ]]; then
  AIR_BIN="./.tmp/bin/air"
else
  echo "air 未安装，推荐先执行: make tools" >&2
  echo "或手动安装: go install github.com/air-verse/air@latest（确保 GOPATH/bin 在 PATH）" >&2
  exit 1
fi

if [[ ! -f "./.env" ]]; then
  cp "./.env.example" "./.env"
  echo "已生成 .env（来自 .env.example）"
fi

set -a
if [[ -f "./.env" ]]; then
  # shellcheck disable=SC1091
  source "./.env"
fi
set +a

# 本地开发：固定 8080 + 正常模式（非 self_mode）
export REALMS_ENV="dev"
export REALMS_ADDR=":8080"
export REALMS_SELF_MODE_ENABLE="false"

exec "${AIR_BIN}"
