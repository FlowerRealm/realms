#!/usr/bin/env bash
set -euo pipefail

ADDR="${REALMS_SMOKE_ADDR:-127.0.0.1:19090}"
BASE_URL="http://${ADDR}"

cleanup() {
  if [[ "${REALMS_E2E_PID:-}" != "" ]]; then
    kill "${REALMS_E2E_PID}" >/dev/null 2>&1 || true
    wait "${REALMS_E2E_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

REALMS_E2E_ADDR="${ADDR}" go run ./cmd/realms-e2e >/tmp/realms-smoke.log 2>&1 &
REALMS_E2E_PID="$!"

echo "[smoke] starting realms-e2e on ${BASE_URL} (pid=${REALMS_E2E_PID})"

for _ in $(seq 1 50); do
  if curl -fsS "${BASE_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

echo "[smoke] GET /healthz"
curl -fsS "${BASE_URL}/healthz"
echo

echo "[smoke] POST /v1/responses"
curl -fsS "${BASE_URL}/v1/responses" \
  -H "Authorization: Bearer sk_playwright_e2e_user_token" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.2","input":"hello","stream":false}'
echo

echo "[smoke] OK"
