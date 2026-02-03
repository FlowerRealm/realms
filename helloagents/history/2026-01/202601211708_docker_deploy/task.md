# 任务清单: Docker 部署（docker compose）

目录: `helloagents/plan/202601211708_docker_deploy/`

---

## 1. 部署（docker compose）
- [√] 1.1 在 `docker-compose.yml` 中新增 `realms` 服务：构建镜像、端口映射、环境变量覆盖，验证 why.md#需求-docker-compose-deploy-场景-up-and-running
- [√] 1.2 在 `docker-compose.yml` 中为 `mysql` 增加数据卷持久化（命名卷挂载 `/var/lib/mysql`），验证 why.md#需求-docker-compose-deploy-场景-up-and-running
- [√] 1.3 增加 `.env.example`，包含部署所需的最小变量（如 `MYSQL_ROOT_PASSWORD`、`MYSQL_DATABASE`、`REALMS_DB_DSN`、`REALMS_ENV`、`REALMS_DISABLE_SECURE_COOKIES`），验证 why.md#需求-docker-compose-deploy-场景-up-and-running

## 2. 安全检查
- [√] 2.1 确认 `.env` 未入库（仓库已有 `.gitignore`），并在文档中提示不要提交真实密钥（按G9）

## 3. 文档更新
- [√] 3.1 更新 `README.md`：补充 Docker 部署步骤（`.env` + `docker compose up -d --build`）与无 TLS 场景的 Cookie 注意事项，验证 why.md#需求-docker-compose-deploy-场景-up-and-running

## 4. 测试
- [√] 4.1 本地执行 `docker compose up -d --build` 并验证 `docker compose ps` / `docker logs realms` 正常
- [√] 4.2 执行一次 `docker compose down` 后再次 `docker compose up -d`，验证 MySQL 数据卷仍存在（简单插入/迁移结果不丢失），验证 why.md#需求-docker-compose-deploy-场景-up-and-running
