#!/usr/bin/env bash
set -euo pipefail

# mysql-capture-lockwaits.sh: capture InnoDB lock-wait evidence for rollup hotspot analysis.
#
# Usage (run while load test is ongoing):
#   REALMS_EVIDENCE_MYSQL_HOST=127.0.0.1 REALMS_EVIDENCE_MYSQL_PORT=3306 \
#   REALMS_EVIDENCE_MYSQL_USER=root REALMS_EVIDENCE_MYSQL_PASS=root REALMS_EVIDENCE_MYSQL_DB=realms \
#   REALMS_EVIDENCE_SECONDS=30 bash scripts/mysql-capture-lockwaits.sh
#
# Output: writes a timestamped log under ./output/

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v mysql >/dev/null 2>&1; then
  echo "mysql CLI not found" >&2
  exit 2
fi

HOST="${REALMS_EVIDENCE_MYSQL_HOST:-127.0.0.1}"
PORT="${REALMS_EVIDENCE_MYSQL_PORT:-3306}"
USER="${REALMS_EVIDENCE_MYSQL_USER:-root}"
PASS="${REALMS_EVIDENCE_MYSQL_PASS:-root}"
DB="${REALMS_EVIDENCE_MYSQL_DB:-realms}"
SECS="${REALMS_EVIDENCE_SECONDS:-30}"

ts="$(date +%Y%m%d-%H%M%S)"
out="output/mysql-lockwaits-${ts}.log"
mkdir -p output

MYSQL=(mysql -h"${HOST}" -P"${PORT}" -u"${USER}" "-p${PASS}" --protocol=tcp --silent --skip-column-names "${DB}")

{
  echo "[capture] ts=${ts} host=${HOST}:${PORT} db=${DB} seconds=${SECS}"
  echo "[capture] NOTE: run this while a high-concurrency finalize/load is running."
  echo

  echo "== SHOW ENGINE INNODB STATUS (header) =="
  mysql -h"${HOST}" -P"${PORT}" -u"${USER}" "-p${PASS}" --protocol=tcp -e "SHOW ENGINE INNODB STATUS\\G" 2>/dev/null | sed -n '1,200p' || true
  echo

  echo "== performance_schema summary (if available) =="
  "${MYSQL[@]}" -e "SELECT @@version, @@performance_schema;" 2>/dev/null || true
  echo

  t_end="$(( $(date +%s) + SECS ))"
  i=0
  while [[ "$(date +%s)" -lt "${t_end}" ]]; do
    i="$(( i + 1 ))"
    echo "-- sample ${i} @ $(date -Is)"

    # MySQL 8+: data_lock_waits / data_locks
    "${MYSQL[@]}" -e "
SELECT COUNT(1)
FROM performance_schema.data_lock_waits w
JOIN performance_schema.data_locks b ON b.engine_lock_id=w.blocking_engine_lock_id
WHERE b.object_name LIKE 'usage_rollup%';
" 2>/dev/null || true

    "${MYSQL[@]}" -e "
SELECT
  b.object_name,
  COUNT(1) AS waits
FROM performance_schema.data_lock_waits w
JOIN performance_schema.data_locks b ON b.engine_lock_id=w.blocking_engine_lock_id
WHERE b.object_name LIKE 'usage_rollup%'
GROUP BY b.object_name
ORDER BY waits DESC
LIMIT 10;
" 2>/dev/null || true

    # Fallback: sys schema (if present)
    "${MYSQL[@]}" -e "
SELECT object_name, COUNT(1) waits
FROM sys.innodb_lock_waits
WHERE object_name LIKE 'usage_rollup%'
GROUP BY object_name
ORDER BY waits DESC
LIMIT 10;
" 2>/dev/null || true

    echo
    sleep 1
  done

  echo "== SHOW ENGINE INNODB STATUS (tail) =="
  mysql -h"${HOST}" -P"${PORT}" -u"${USER}" "-p${PASS}" --protocol=tcp -e "SHOW ENGINE INNODB STATUS\\G" 2>/dev/null | tail -n 200 || true
} | tee "${out}"

echo "[capture] wrote ${out}"

