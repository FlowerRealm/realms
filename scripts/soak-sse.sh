#!/usr/bin/env bash
set -euo pipefail

ADDR="${REALMS_SOAK_ADDR:-127.0.0.1:19090}"
BASE_URL="http://${ADDR}"
CONNS="${REALMS_SOAK_CONNS:-200}"
DUR="${REALMS_SOAK_DURATION:-10s}"
RAMP="${REALMS_SOAK_RAMP:-2s}"

cleanup() {
  if [[ "${REALMS_E2E_PID:-}" != "" ]]; then
    kill "${REALMS_E2E_PID}" >/dev/null 2>&1 || true
    wait "${REALMS_E2E_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

REALMS_E2E_ADDR="${ADDR}" go run ./cmd/realms-e2e >/tmp/realms-soak-sse.log 2>&1 &
REALMS_E2E_PID="$!"

echo "[soak-sse] starting realms-e2e on ${BASE_URL} (pid=${REALMS_E2E_PID})"
for _ in $(seq 1 50); do
  if curl -fsS "${BASE_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

go run ./cmd/realms-load-sse \
  -base-url "${BASE_URL}" \
  -conns "${CONNS}" \
  -duration "${DUR}" \
  -ramp "${RAMP}"

