#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if [[ "${REALMS_DEV_MYSQL:-}" =~ ^(0|false|no|off|skip)$ ]]; then
  exit 0
fi

MYSQL_HOST="${REALMS_DEV_MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${REALMS_DEV_MYSQL_PORT:-3306}"
MYSQL_SERVICE="${REALMS_DEV_MYSQL_SERVICE:-mysql}"

is_listening() {
  local host="$1"
  local port="$2"
  (exec 3<>"/dev/tcp/${host}/${port}") >/dev/null 2>&1
}

# 只对本地开发常见配置自动拉起容器；非本机地址交由用户自行管理。
if [[ "${MYSQL_HOST}" != "127.0.0.1" && "${MYSQL_HOST}" != "localhost" ]]; then
  exit 0
fi

if is_listening "${MYSQL_HOST}" "${MYSQL_PORT}"; then
  echo ">> MySQL 已在 ${MYSQL_HOST}:${MYSQL_PORT} 监听，跳过 docker compose"
  exit 0
fi

if [[ ! -f "./docker-compose.yml" ]]; then
  echo ">> 未找到 docker-compose.yml，跳过启动 docker MySQL（将继续尝试连接 ${MYSQL_HOST}:${MYSQL_PORT}）" >&2
  exit 0
fi

DOCKER_COMPOSE=(docker compose)
if docker compose version >/dev/null 2>&1; then
  :
elif command -v docker-compose >/dev/null 2>&1; then
  DOCKER_COMPOSE=(docker-compose)
else
  echo ">> 未找到 docker compose（docker compose / docker-compose），跳过启动 docker MySQL" >&2
  exit 0
fi

echo ">> 启动 MySQL（docker compose: ${MYSQL_SERVICE}）"
if ! "${DOCKER_COMPOSE[@]}" up -d "${MYSQL_SERVICE}"; then
  echo "!! 启动 docker MySQL 失败" >&2
  echo "   - 如果本机 3306 已被占用：请修改 docker-compose.yml 端口映射或复用现有 MySQL" >&2
  exit 1
fi

