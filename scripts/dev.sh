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

should_start_mysql=0

if [[ "${REALMS_DB_DRIVER:-}" =~ ^mysql$ ]]; then
  should_start_mysql=1
elif [[ "${REALMS_DB_DRIVER:-}" =~ ^sqlite$ ]]; then
  should_start_mysql=0
elif [[ -n "${REALMS_DB_DSN:-}" ]]; then
  # 兼容旧配置：仅设置 db.dsn 时推断为 mysql。
  should_start_mysql=1
elif [[ -f "./config.yaml" ]]; then
  if grep -Eq '^[[:space:]]*driver:[[:space:]]*"?mysql"?' "./config.yaml"; then
    should_start_mysql=1
  elif grep -Eq '^[[:space:]]*dsn:[[:space:]]*"?[^"#[:space:]]' "./config.yaml"; then
    should_start_mysql=1
  fi
fi

if [[ "${should_start_mysql}" -eq 1 ]]; then
  bash "./scripts/dev-mysql.sh"
fi

exec "${AIR_BIN}"
