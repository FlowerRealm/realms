#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

ADDR="${REALMS_SMOKE_ADDR:-127.0.0.1:19090}"
BASE_URL="http://${ADDR}"
API_BASE="${BASE_URL}/v1"
MODEL="${REALMS_SMOKE_MODEL:-gpt-5.2}"
TOKEN="${REALMS_SMOKE_TOKEN:-sk_e2e_user_token}"

cleanup() {
  if [[ "${REALMS_E2E_PID:-}" != "" ]]; then
    kill "${REALMS_E2E_PID}" >/dev/null 2>&1 || true
    wait "${REALMS_E2E_PID}" >/dev/null 2>&1 || true
  fi
  if [[ "${REALMS_SMOKE_TMP_DIR:-}" != "" ]]; then
    case "${REALMS_SMOKE_TMP_DIR}" in
      "${ROOT_DIR}/.tmp/realms-smoke-codex."*|/tmp/realms-smoke-codex.*)
        rm -r -- "${REALMS_SMOKE_TMP_DIR}" >/dev/null 2>&1 || true
        ;;
    esac
  fi
}
trap cleanup EXIT

if ! command -v codex >/dev/null 2>&1; then
  echo "[smoke-codex] missing codex in PATH (hint: npm install -g @openai/codex)" >&2
  exit 2
fi

REALMS_E2E_ADDR="${ADDR}" go run ./cmd/realms-e2e >/tmp/realms-smoke-codex.log 2>&1 &
REALMS_E2E_PID="$!"

echo "[smoke-codex] starting realms-e2e on ${BASE_URL} (pid=${REALMS_E2E_PID})"

for _ in $(seq 1 50); do
  if curl -fsS "${BASE_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

echo "[smoke-codex] GET /healthz"
curl -fsS "${BASE_URL}/healthz"
echo

mkdir -p "${ROOT_DIR}/.tmp"
REALMS_SMOKE_TMP_DIR="$(mktemp -d "${ROOT_DIR}/.tmp/realms-smoke-codex.XXXXXX")"
home_dir="${REALMS_SMOKE_TMP_DIR}/home"
work_dir="${REALMS_SMOKE_TMP_DIR}/work"
mkdir -p "${home_dir}/.codex" "${work_dir}"

cat > "${home_dir}/.codex/config.toml" <<EOF
disable_response_storage = true
model_provider = "realms"
model = "${MODEL}"

[model_providers.realms]
name = "Realms"
base_url = "${API_BASE}"
wire_api = "responses"
requires_openai_auth = true
env_key = "OPENAI_API_KEY"
EOF

echo "[smoke-codex] codex exec -> ${API_BASE}"
prompt="Reply with exactly: OK"

set +e
out="$(
  HOME="${home_dir}" \
  OPENAI_API_KEY="${TOKEN}" \
  CODEX_API_KEY="" \
  codex exec --skip-git-repo-check "${prompt}" 2>&1
)"
rc="$?"
set -e

if [[ "${rc}" -ne 0 ]]; then
  echo "[smoke-codex] codex exec failed (rc=${rc})" >&2
  echo "${out}" >&2
  exit 1
fi

if ! grep -q "OK" <<<"${out}"; then
  echo "[smoke-codex] unexpected codex output (missing OK)" >&2
  echo "${out}" >&2
  exit 1
fi

echo "[smoke-codex] OK"
