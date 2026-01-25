#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v git >/dev/null 2>&1; then
  echo "[ERROR] 未找到 git"
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "[ERROR] 未找到 docker"
  exit 1
fi

DOCKER="docker"
if ! docker ps >/dev/null 2>&1; then
  DOCKER="sudo docker"
fi

ts="$(date +%F_%H%M%S)"
mkdir -p backups
chmod 700 backups

echo "[INFO] 备份配置与数据库..."
if [ -f .env ]; then
  cp .env "backups/.env.$ts"
  chmod 600 "backups/.env.$ts"
fi

if $DOCKER compose ps -q mysql >/dev/null 2>&1 && [ -n "$($DOCKER compose ps -q mysql)" ]; then
  $DOCKER compose exec -T mysql sh -lc 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --databases "$MYSQL_DATABASE" --single-transaction --quick --set-gtid-purged=OFF' \
    | gzip > "backups/mysql.$ts.sql.gz"
  chmod 600 "backups/mysql.$ts.sql.gz"
else
  echo "[WARN] 未检测到 mysql 容器在运行，跳过数据库备份"
fi

echo "[INFO] 更新代码（git rebase onto origin/master）..."
if [ -n "$(git status --porcelain)" ]; then
  echo "[ERROR] 工作区存在未提交修改，请先处理后再更新："
  git status -sb
  exit 1
fi

before="$(git rev-parse --short HEAD)"
git fetch origin master
git rebase origin/master
after="$(git rev-parse --short HEAD)"

echo "[INFO] 重建并重启（docker compose up -d --build）..."
build_commit="$(git rev-parse --short HEAD 2>/dev/null || echo "none")"
build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
REALMS_COMMIT="${build_commit}" REALMS_BUILD_DATE="${build_date}" $DOCKER compose up -d --build

port="18080"
if [ -f .env ]; then
  port="$(awk -F= '/^REALMS_HTTP_PORT=/{print $2}' .env | tail -n 1)"
  port="${port:-18080}"
fi

echo "[INFO] 健康检查..."
curl -fsS "http://127.0.0.1:${port}/healthz" >/dev/null

echo "[SUCCESS] 更新完成：$before -> $after"
echo "[INFO] 查看日志：$DOCKER compose logs -f realms"
