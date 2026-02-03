# 变更提案: Docker 部署（docker compose）

## 需求背景
当前项目已提供 `Dockerfile`（构建 `realms` 二进制），但 `docker-compose.yml` 仅包含 MySQL，无法做到“一条命令启动整套服务”用于长期运行部署。

本变更目标是在不引入反向代理（无 TLS 终止）的前提下，补全基于 `docker compose` 的全栈部署：`realms + mysql`，并使用环境变量覆盖 Realms 配置，且仅要求持久化 MySQL 数据。

## 变更内容
1. 扩展 `docker-compose.yml`：新增 `realms` 服务并与 `mysql` 组网，支持一键启动
2. 引入 `.env.example`：提供部署所需的环境变量示例（不提交真实密钥）
3. 更新 `README.md`：补充 Docker 部署运行说明与注意事项

## 影响范围
- **模块:** 部署/运维
- **文件:**
  - `docker-compose.yml`
  - `.env.example`
  - `README.md`
- **API:** 无变更
- **数据:** 仅新增 MySQL 数据卷持久化（不引入额外持久化目录）

## 核心场景

### 需求: docker-compose-deploy
**模块:** 部署
通过 `docker compose` 启动 `realms + mysql`，Realms 通过环境变量配置数据库连接等关键项。

#### 场景: up-and-running
在一台主机上执行 `docker compose up -d --build` 后：
- MySQL 正常启动并通过 healthcheck
- Realms 在 MySQL 就绪后启动成功并监听 `:8080`
- 重启容器后仍能正常工作，且 MySQL 数据卷不丢失

#### 场景: upgrade
当更新代码后执行重新构建与重启：
- Realms 可重新构建镜像并滚动重启
- 启动时自动执行内置迁移（如有），MySQL 数据不丢失

## 风险评估
- **风险:** 未使用反向代理/TLS，Web 登录 Cookie 可能需要允许非 Secure Cookie 才能在 HTTP 下工作
  - **缓解:** 在部署文档中明确提示仅建议内网/受信环境使用；通过环境变量显式开启 `disable_secure_cookies`
- **风险:** MySQL 启动竞态导致 Realms 启动失败（`env=prod` 仅 ping 一次）
  - **缓解:** `docker compose` 使用 `depends_on` + MySQL healthcheck，确保 MySQL 就绪后再启动 Realms
- **风险:** 密钥/口令通过环境变量配置可能被误提交
  - **缓解:** 使用 `.env`（已被 `.gitignore` 忽略）并提供 `.env.example` 作为模板

