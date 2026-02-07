#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

AIR_BIN="air"
FRONTEND_WATCH_PID=""
FRONTEND_WATCH_OWN_GROUP="false"
AIR_PID=""

stop_frontend_watch() {
  if [[ -z "${FRONTEND_WATCH_PID}" ]]; then
    return
  fi
  if [[ "${FRONTEND_WATCH_OWN_GROUP}" == "true" ]]; then
    kill -- -"${FRONTEND_WATCH_PID}" >/dev/null 2>&1 || true
  else
    kill "${FRONTEND_WATCH_PID}" >/dev/null 2>&1 || true
    pkill -P "${FRONTEND_WATCH_PID}" >/dev/null 2>&1 || true
  fi
  for _ in {1..20}; do
    if ! kill -0 "${FRONTEND_WATCH_PID}" >/dev/null 2>&1; then
      break
    fi
    sleep 0.1
  done
  if kill -0 "${FRONTEND_WATCH_PID}" >/dev/null 2>&1; then
    if [[ "${FRONTEND_WATCH_OWN_GROUP}" == "true" ]]; then
      kill -9 -- -"${FRONTEND_WATCH_PID}" >/dev/null 2>&1 || true
    else
      kill -9 "${FRONTEND_WATCH_PID}" >/dev/null 2>&1 || true
      pkill -9 -P "${FRONTEND_WATCH_PID}" >/dev/null 2>&1 || true
    fi
  fi
  wait "${FRONTEND_WATCH_PID}" >/dev/null 2>&1 || true
  FRONTEND_WATCH_PID=""
  FRONTEND_WATCH_OWN_GROUP="false"
}

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  stop_frontend_watch
  if [[ -n "${AIR_PID}" ]] && kill -0 "${AIR_PID}" >/dev/null 2>&1; then
    kill "${AIR_PID}" >/dev/null 2>&1 || true
    wait "${AIR_PID}" >/dev/null 2>&1 || true
  fi
  exit "${exit_code}"
}

trap cleanup EXIT INT TERM

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

# dev 默认需要前端构建产物（web/dist）；否则后端静态资源会缺失
FRONTEND_DIST_DIR="${FRONTEND_DIST_DIR:-./web/dist}"
if [[ "${FRONTEND_DIST_DIR}" == "./web/dist" || "${FRONTEND_DIST_DIR}" == "web/dist" ]]; then
  if ! command -v npm >/dev/null 2>&1; then
    echo "未找到 npm，无法启动前端 watch。请先安装 Node.js/npm。" >&2
    exit 1
  fi

  if [[ ! -d "./web/node_modules" ]]; then
    echo ">> installing frontend deps (npm ci)"
    npm --prefix "./web" ci
  fi

  if [[ ! -d "./web/dist" ]] || ! compgen -G "./web/dist/*" >/dev/null; then
    echo ">> building frontend (npm run build)"
    npm --prefix "./web" run build
  fi

  echo ">> starting frontend watch (npm run build -- --watch)"
  if command -v setsid >/dev/null 2>&1; then
    setsid npm --prefix "./web" run build -- --watch &
    FRONTEND_WATCH_OWN_GROUP="true"
  else
    npm --prefix "./web" run build -- --watch &
    FRONTEND_WATCH_OWN_GROUP="false"
  fi
  FRONTEND_WATCH_PID=$!
  sleep 1
  if ! kill -0 "${FRONTEND_WATCH_PID}" >/dev/null 2>&1; then
    echo "前端 watch 启动失败，请检查 npm 输出。" >&2
    wait "${FRONTEND_WATCH_PID}" || true
    exit 1
  fi
else
  echo "检测到 FRONTEND_DIST_DIR=${FRONTEND_DIST_DIR}（非 ./web/dist），跳过自动前端构建" >&2
fi

# 本地开发：固定 8080 + 正常模式（非 self_mode）
export REALMS_ENV="dev"
export REALMS_ADDR=":8080"
export REALMS_SELF_MODE_ENABLE="false"

"${AIR_BIN}" &
AIR_PID=$!
wait "${AIR_PID}"
