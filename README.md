# Realms

Realms 是一个 Go 单体服务（Gin），对外提供 OpenAI 兼容的 `/v1/*` 数据面，并提供 Web 管理后台用于管理用户、令牌、上游渠道、计费、公告与用量。

## 当前产品形态

- 只保留统一的服务端模式
- 不再支持 `personal` 模式
- 不再提供 `realms-app` 启动器、personal 前端、MCP/Skills 管理页或 personal 管理 Key / personal API Key

## 快速开始

### Docker Compose

```bash
git clone "https://github.com/FlowerRealm/realms.git"
cd "realms"
cp ".env.example" ".env"
docker compose pull realms
docker compose up -d
curl -fsS "http://127.0.0.1:8080/healthz"
```

### 本地开发

```bash
make tools
make dev
```

前端默认构建到 `web/dist`，后端默认监听 `127.0.0.1:8080`。

如果本地开发需要固定连接另一套数据库，不要直接复用 `REALMS_DB_DSN` 去和 Docker Compose 抢同一个目标。请改用 `.env` 里的 `REALMS_DB_DSN_DEV` / `REALMS_DB_DRIVER_DEV` / `REALMS_SQLITE_PATH_DEV`，它们只在 `REALMS_ENV=dev`（例如 `make dev`）时生效。

## 认证模型

### Web 管理面

- 使用账号注册 / 登录
- 第一个注册用户会自动成为 `root`
- 管理类 API 也可使用 `REALMS_ADMIN_API_KEY` 直接访问 `/api/admin/*` 与 `/api/channel*`

### 数据面 `/v1/*`

- 使用 `/tokens` 创建的用户 Token
- 常见环境变量：

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="sk_..."
```

## 关键环境变量

- `REALMS_ENV`
- `REALMS_ADDR`
- `REALMS_DB_DRIVER`
- `REALMS_DB_DSN`
- `REALMS_SQLITE_PATH`
- `REALMS_PUBLIC_BASE_URL`
- `REALMS_CORS_ALLOW_ORIGINS`
- `REALMS_ADMIN_API_KEY`
- `REALMS_COMPACT_GATEWAY_BASE_URL`
- `REALMS_COMPACT_GATEWAY_KEY`
- `REALMS_COMPACT_GATEWAY_TIMEOUT_MS`
- `FRONTEND_DIST_DIR`
- `FRONTEND_BASE_URL`

`/v1/responses/compact` 仅通过 `compact gateway` 转发到远程上游；如果需要启用这条链路，请配置 `REALMS_COMPACT_GATEWAY_*`。旧的 `REALMS_SUB2API_*` 已移除，继续使用会在启动时直接报错。

## 重要变更

- `REALMS_MODE` 已移除；设置该变量会直接报错
- `REALMS_SUB2API_*` 已移除；请改用 `REALMS_COMPACT_GATEWAY_*`
- `cmd/realms-app`、`make app-dev`、`make app-dist`、`make app-set-key` 已删除
- `web/dist-personal`、`npm --prefix web run build:personal` 与 personal embed 产物已删除
