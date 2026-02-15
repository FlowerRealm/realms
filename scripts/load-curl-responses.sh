#!/usr/bin/env bash
set -euo pipefail

# Simple curl-based load tester for POST /v1/responses (non-stream).
#
# Env:
# - REALMS_LOAD_BASE_URL   default: http://127.0.0.1:19090
# - REALMS_LOAD_TOKEN      default: sk_playwright_e2e_user_token
# - REALMS_LOAD_MODEL      default: gpt-5.2
# - REALMS_LOAD_INPUT      default: hello
# - REALMS_LOAD_REQUESTS   default: 200
# - REALMS_LOAD_PARALLEL   default: 20
#
# Output: basic latency stats (ms).

BASE_URL="${REALMS_LOAD_BASE_URL:-http://127.0.0.1:19090}"
TOKEN="${REALMS_LOAD_TOKEN:-sk_playwright_e2e_user_token}"
MODEL="${REALMS_LOAD_MODEL:-gpt-5.2}"
INPUT="${REALMS_LOAD_INPUT:-hello}"
REQS="${REALMS_LOAD_REQUESTS:-200}"
PAR="${REALMS_LOAD_PARALLEL:-20}"

if ! [[ "${REQS}" =~ ^[0-9]+$ ]] || [[ "${REQS}" -le 0 ]]; then
  echo "REALMS_LOAD_REQUESTS must be positive int" >&2
  exit 2
fi
if ! [[ "${PAR}" =~ ^[0-9]+$ ]] || [[ "${PAR}" -le 0 ]]; then
  echo "REALMS_LOAD_PARALLEL must be positive int" >&2
  exit 2
fi

tmp="$(mktemp -t realms-load.XXXXXX)"
sorted="$(mktemp -t realms-load.sorted.XXXXXX)"
cleanup() { rm -f "${tmp}" "${tmp}.ms" "${sorted}"; }
trap cleanup EXIT

echo "[load] base_url=${BASE_URL} requests=${REQS} parallel=${PAR}"

seq 1 "${REQS}" | xargs -P "${PAR}" -I{} bash -c '
  curl -sS -o /dev/null \
    -w "%{time_total}\n" \
    "'"${BASE_URL}"'/v1/responses" \
    -H "Authorization: Bearer '"${TOKEN}"'" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"'"${MODEL}"'\",\"input\":\"'"${INPUT}"'\",\"stream\":false}" \
  ' >"${tmp}"

awk '{ print $1 * 1000.0 }' "${tmp}" > "${tmp}.ms"
sort -n "${tmp}.ms" > "${sorted}"

n="$(wc -l < "${sorted}" | tr -d " ")"
if [[ "${n}" -le 0 ]]; then
  echo "no samples" >&2
  exit 1
fi

avg="$(awk '{ sum += $1 } END { printf("%.2f", sum / NR) }' "${tmp}.ms")"
p50_idx="$(( ( (n - 1) * 50 ) / 100 + 1 ))"
p95_idx="$(( ( (n - 1) * 95 ) / 100 + 1 ))"
p99_idx="$(( ( (n - 1) * 99 ) / 100 + 1 ))"
p50="$(sed -n "${p50_idx}p" "${sorted}")"
p95="$(sed -n "${p95_idx}p" "${sorted}")"
p99="$(sed -n "${p99_idx}p" "${sorted}")"

printf "[load] samples=%d avg_ms=%s p50_ms=%s p95_ms=%s p99_ms=%s\n" "${n}" "${avg}" "${p50}" "${p95}" "${p99}"
