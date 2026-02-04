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

# dev 默认需要前端构建产物（web/dist）；否则后端静态资源会缺失
FRONTEND_DIST_DIR="${FRONTEND_DIST_DIR:-./web/dist}"
if [[ "${FRONTEND_DIST_DIR}" == "./web/dist" || "${FRONTEND_DIST_DIR}" == "web/dist" ]]; then
  if [[ ! -d "./web/dist" ]] || ! compgen -G "./web/dist/*" >/dev/null; then
    if ! command -v npm >/dev/null 2>&1; then
      echo "未找到 npm，无法自动构建前端。请先安装 Node.js/npm，或手动执行: (cd web && npm ci && npm run build)" >&2
      exit 1
    fi
    if [[ ! -d "./web/node_modules" ]]; then
      echo ">> installing frontend deps (npm ci)"
      npm --prefix "./web" ci
    fi
    echo ">> building frontend (npm run build)"
    npm --prefix "./web" run build
  fi
else
  echo "检测到 FRONTEND_DIST_DIR=${FRONTEND_DIST_DIR}（非 ./web/dist），跳过自动前端构建" >&2
fi

# 本地开发：固定 8080 + 正常模式（非 self_mode）
export REALMS_ENV="dev"
export REALMS_ADDR=":8080"
export REALMS_SELF_MODE_ENABLE="false"

exec "${AIR_BIN}"
