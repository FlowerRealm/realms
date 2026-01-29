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

is_disabled() {
  local v="${1:-}"
  [[ "${v}" =~ ^(0|false|no|off|skip)$ ]]
}

start_docker_self_mode() {
  if is_disabled "${REALMS_DEV_DOCKER_SELF:-}"; then
    return 0
  fi

  local project="${REALMS_DEV_DOCKER_PROJECT:-realms-dev-self}"
  local http_port="${REALMS_DEV_DOCKER_HTTP_PORT:-7080}"
  local mysql_host_port="${REALMS_DEV_DOCKER_MYSQL_HOST_PORT:-7306}"
  local mysql_bind_ip="${REALMS_DEV_DOCKER_MYSQL_BIND_IP:-0.0.0.0}"

  local compose=(docker compose)
  if docker compose version >/dev/null 2>&1; then
    :
  elif command -v docker-compose >/dev/null 2>&1; then
    compose=(docker-compose)
  else
    echo ">> 未找到 docker compose（docker compose / docker-compose），跳过 Docker self_mode（仍启动本地）" >&2
    return 0
  fi

  echo ">> 启动 Docker self_mode（${project}）：http://127.0.0.1:${http_port}（MySQL: ${mysql_bind_ip}:${mysql_host_port}）"
  # 显式指定 -f，避免自动加载 docker-compose.override.yml（常用于复用外部 mysql_data 卷，会破坏“数据库隔离”）。
  if ! REALMS_SELF_MODE_ENABLE=true REALMS_HTTP_PORT="${http_port}" MYSQL_HOST_PORT="${mysql_host_port}" MYSQL_BIND_IP="${mysql_bind_ip}" "${compose[@]}" -f "./docker-compose.yml" -p "${project}" up -d mysql realms; then
    echo "!! 启动 Docker self_mode 失败（仍继续启动本地）" >&2
  fi
}

start_docker_self_mode

# 本地开发：固定 8080 + 正常模式（非 self_mode）
export REALMS_ADDR=":8080"
export REALMS_SELF_MODE_ENABLE="false"

should_start_mysql=0

if [[ "${REALMS_DB_DRIVER:-}" =~ ^mysql$ ]]; then
  should_start_mysql=1
elif [[ "${REALMS_DB_DRIVER:-}" =~ ^sqlite$ ]]; then
  should_start_mysql=0
elif [[ -n "${REALMS_DB_DSN:-}" ]]; then
  # 兼容旧配置：仅设置 db.dsn 时推断为 mysql。
  should_start_mysql=1
fi

if [[ "${should_start_mysql}" -eq 1 ]]; then
  bash "./scripts/dev-mysql.sh"
fi

exec "${AIR_BIN}"
