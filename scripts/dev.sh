#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

AIR_BIN="air"
FRONTEND_WATCH_PID=""
FRONTEND_WATCH_OWN_GROUP="false"
AIR_PID=""

trim_leading_whitespace() {
  local value="$1"
  printf '%s' "${value#"${value%%[![:space:]]*}"}"
}

trim_trailing_whitespace() {
  local value="$1"
  printf '%s' "${value%"${value##*[![:space:]]}"}"
}

trim_whitespace() {
  local value="$1"
  value="$(trim_leading_whitespace "${value}")"
  trim_trailing_whitespace "${value}"
}

strip_inline_comment() {
  local value="$1"
  local result=""
  local quote=""
  local prev=""
  local ch=""
  local i=0

  for ((i = 0; i < ${#value}; i++)); do
    ch="${value:i:1}"
    if [[ -n "${quote}" ]]; then
      if [[ "${ch}" == "${quote}" && "${prev}" != "\\" ]]; then
        quote=""
      fi
      result+="${ch}"
    else
      case "${ch}" in
        "'"|'"')
          quote="${ch}"
          result+="${ch}"
          ;;
        "#")
          if [[ -z "${prev}" || "${prev}" =~ [[:space:]] ]]; then
            break
          fi
          result+="${ch}"
          ;;
        *)
          result+="${ch}"
          ;;
      esac
    fi
    prev="${ch}"
  done

  trim_trailing_whitespace "${result}"
}

unquote_dotenv_value() {
  local value="$1"

  if [[ "${value}" =~ ^\".*\"$ ]]; then
    value="${value:1:${#value}-2}"
    value="${value//\\\"/\"}"
    printf '%s' "${value}"
    return
  fi

  if [[ "${value}" =~ ^\'.*\'$ ]]; then
    printf '%s' "${value:1:${#value}-2}"
    return
  fi

  printf '%s' "${value}"
}

load_dotenv_file() {
  local env_file="$1"
  local line=""
  local key=""
  local value=""

  [[ -f "${env_file}" ]] || return 0

  # .env 是配置数据，不是 shell 脚本；这里按 dotenv 键值对字面解析。
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%$'\r'}"
    line="$(trim_whitespace "${line}")"

    if [[ -z "${line}" || "${line}" == \#* ]]; then
      continue
    fi

    if [[ "${line}" == export[[:space:]]* ]]; then
      line="$(trim_leading_whitespace "${line#export}")"
    fi

    if [[ "${line}" != *=* ]]; then
      continue
    fi

    key="$(trim_whitespace "${line%%=*}")"
    value="${line#*=}"

    if [[ ! "${key}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
      continue
    fi

    value="$(strip_inline_comment "${value}")"
    value="$(trim_leading_whitespace "${value}")"
    value="$(unquote_dotenv_value "${value}")"
    export "${key}=${value}"
  done <"${env_file}"
}

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

load_dotenv_file "./.env"

# dev：默认启用 CLI 渠道测试，并尽力拉起本地 cli-runner（docker compose）。
if [[ ! "${REALMS_DEV_CLI_RUNNER:-}" =~ ^(0|false|no|off|skip)$ ]]; then
  if [[ -z "${REALMS_CHANNEL_TEST_CLI_RUNNER_URL:-}" ]]; then
    export REALMS_CHANNEL_TEST_CLI_RUNNER_URL="http://127.0.0.1:${CLI_RUNNER_PORT:-3100}"
  fi
  bash "./scripts/dev-cli-runner.sh"
fi

# dev 默认需要前端构建产物（web/dist）；否则后端静态资源会缺失
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

# 本地开发：固定 8080
export REALMS_ENV="dev"
export REALMS_ADDR=":8080"

"${AIR_BIN}" &
AIR_PID=$!
wait "${AIR_PID}"
