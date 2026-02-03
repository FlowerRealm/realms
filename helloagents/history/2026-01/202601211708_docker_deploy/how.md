# 技术设计: Docker 部署（docker compose）

## 技术方案

### 核心技术
- Docker / docker compose
- Go 二进制容器化（复用现有 `Dockerfile`）
- MySQL 8.x（复用现有 compose 服务）

### 实现要点
- **Compose 一键启动**
  - 在 `docker-compose.yml` 新增 `realms` 服务，使用 `Dockerfile` 构建镜像
  - `realms` 暴露 `8080` 到宿主机，保持与当前文档一致
  - `realms` 通过环境变量覆盖关键配置（至少 `REALMS_DB_DSN`），不依赖挂载 `config.yaml`
- **MySQL 就绪控制**
  - 保留/使用 MySQL `healthcheck`
  - `realms` 使用 `depends_on: condition: service_healthy`，避免 `env=prod` 下的启动竞态
- **数据持久化**
  - 使用命名卷持久化 `/var/lib/mysql`
  - 不额外挂载 Realms 运行数据目录（按需求仅持久化 MySQL）
- **环境变量约定**
  - 通过 `.env` 文件（不入库）提供 MySQL 口令与 Realms DSN 等值
  - 提供 `.env.example` 作为模板，便于部署侧复制修改

## 安全与性能
- **安全**
  - 不提交任何真实口令/密钥：只提交 `.env.example`，运行时使用 `.env`（仓库已忽略）
  - 无 TLS/反向代理场景下，浏览器不会发送 Secure Cookie：如需使用 Web 登录功能，需要显式设置 `REALMS_DISABLE_SECURE_COOKIES=true`
  - 如需公网部署，应补充反向代理/TLS 终止与 `Secure Cookie`（本需求明确不做）
- **性能**
  - Realms 以静态二进制运行（distroless），启动与资源占用较轻
  - MySQL 使用持久化卷，避免容器重建导致数据重置

## 测试与部署
- **部署流程（期望）**
  1. `cp .env.example .env`，按需修改变量（尤其是 `REALMS_DB_DSN`）
  2. `docker compose up -d --build`
  3. 验证 `http://localhost:8080/` 可访问；并检查 `docker compose ps` 状态正常
- **回归验证点**
  - `mysql` healthcheck 为 healthy 后 `realms` 才启动
  - `realms` 启动后能连接数据库并自动执行迁移
  - 停止/重启容器后 MySQL 数据仍存在（卷持久化生效）

