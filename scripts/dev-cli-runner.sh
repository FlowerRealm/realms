#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if [[ "${REALMS_DEV_CLI_RUNNER:-}" =~ ^(0|false|no|off|skip)$ ]]; then
  exit 0
fi

CLI_RUNNER_HOST="${REALMS_DEV_CLI_RUNNER_HOST:-127.0.0.1}"
CLI_RUNNER_PORT="${REALMS_DEV_CLI_RUNNER_PORT:-${CLI_RUNNER_PORT:-3100}}"
CLI_RUNNER_SERVICE="${REALMS_DEV_CLI_RUNNER_SERVICE:-cli-runner}"

is_listening() {
  local host="$1"
  local port="$2"
  (exec 3<>"/dev/tcp/${host}/${port}") >/dev/null 2>&1
}

# 尝试从 REALMS_CHANNEL_TEST_CLI_RUNNER_URL 推导 host/port，以避免“URL 与 compose 端口映射不一致”。
if [[ -n "${REALMS_CHANNEL_TEST_CLI_RUNNER_URL:-}" ]]; then
  if [[ "${REALMS_CHANNEL_TEST_CLI_RUNNER_URL}" =~ ^https?://([^/:]+)(:([0-9]+))?(/.*)?$ ]]; then
    parsed_host="${BASH_REMATCH[1]}"
    parsed_port="${BASH_REMATCH[3]:-}"
    if [[ -n "${parsed_host}" ]]; then
      CLI_RUNNER_HOST="${parsed_host}"
    fi
    if [[ -n "${parsed_port}" ]]; then
      CLI_RUNNER_PORT="${parsed_port}"
      export CLI_RUNNER_PORT
    fi
  fi
fi

# 只对本地开发常见配置自动拉起容器；非本机地址交由用户自行管理。
if [[ "${CLI_RUNNER_HOST}" != "127.0.0.1" && "${CLI_RUNNER_HOST}" != "localhost" ]]; then
  exit 0
fi

if is_listening "${CLI_RUNNER_HOST}" "${CLI_RUNNER_PORT}"; then
  exit 0
fi

if [[ ! -f "./docker-compose.yml" ]]; then
  echo ">> 未找到 docker-compose.yml，跳过启动 cli-runner（将继续使用 REALMS_CHANNEL_TEST_CLI_RUNNER_URL=${REALMS_CHANNEL_TEST_CLI_RUNNER_URL:-}）" >&2
  exit 0
fi

DOCKER_COMPOSE=(docker compose)
if docker compose version >/dev/null 2>&1; then
  :
elif command -v docker-compose >/dev/null 2>&1; then
  DOCKER_COMPOSE=(docker-compose)
else
  echo ">> 未找到 docker compose（docker compose / docker-compose），跳过启动 cli-runner" >&2
  exit 0
fi

echo ">> 启动 CLI Runner（docker compose: ${CLI_RUNNER_SERVICE}）"
if ! "${DOCKER_COMPOSE[@]}" up -d "${CLI_RUNNER_SERVICE}"; then
  echo "!! 启动 docker cli-runner 失败" >&2
  echo "   - 如果本机端口已被占用：请在 .env 设置 CLI_RUNNER_PORT=13100（或复用现有 runner）" >&2
  exit 1
fi
