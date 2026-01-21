# Realms（OpenAI 风格 API 中转）

Realms 是一个 Go 单体服务（`net/http`），对外提供 **OpenAI 兼容** 的 API（数据面），并提供一个 Web 控制台（管理面）用于配置上游与下游 Token。

**你可以用它做什么：**
- 作为 OpenAI SDK / Codex CLI 的 `base_url` 中转层（支持 `POST /v1/responses` SSE 透传）
- 在 Web 控制台里管理用户 Token（`rlm_...`）、查看用量与请求明细
- 在管理后台里管理上游渠道（OpenAI 兼容 base_url / Codex OAuth）与路由策略

## 文档

- 配置示例：[`config.example.yaml`](config.example.yaml)
- 贡献指南：[`CONTRIBUTING.md`](CONTRIBUTING.md)
- 安全政策：[`SECURITY.md`](SECURITY.md)
- 行为准则：[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)
- 许可证：[`LICENSE`](LICENSE)

## 1) 快速开始（本地）

### 前置

- Go 1.22+
- MySQL 8.x（或兼容）

### 1. 启动 MySQL（本地开发）

`make dev` 会在检测到 `127.0.0.1:3306` 未监听时，自动尝试用 docker compose 启动 MySQL 容器。  
如需禁用该行为，可在环境变量或 `.env` 中设置：`REALMS_DEV_MYSQL=skip`。

如需手动启动：

```bash
docker compose up -d mysql
```

> 提示：如果你的机器上 **3306 已被其他 MySQL 占用**，`docker-compose.yml` 的端口映射会冲突。  
> 这时可以：
> 1) 复用现有 MySQL（确保存在 `realms` 数据库）；或  
> 2) 修改 `docker-compose.yml` 将宿主端口改为其他端口（如 `13306`），并同步更新 `config.yaml` 的 `db.dsn`。

### 2. 启动 Realms

```bash
cp config.example.yaml config.yaml
go run ./cmd/realms -config config.yaml
```

> 说明：默认配置文件路径为 `config.yaml`，因此也可以直接运行编译产物：`./realms` 或容器内 `/realms`。

首次启动会自动执行内置迁移（`internal/store/migrations/*.sql`）。  
在 `env=dev` 且账号具备权限时，如果目标数据库不存在，会自动创建数据库后继续迁移。  
如果 MySQL 处于启动过程中（常见于刚 `docker compose up`），dev 环境会等待 MySQL 就绪（最多 30s）后再继续。

### 3. 配置上游（必做）

Realms 本身不自带可用上游，启动后请先完成一次上游配置，否则数据面无法转发请求。

1) 打开 Web 控制台：`http://localhost:8080/`  
2) 注册并登录（开发期默认允许注册：`security.allow_open_registration=true`）  
3) **第一个注册的用户会被设置为 `root`**  
4) 进入管理后台：`http://localhost:8080/admin`

在管理后台配置上游：
- OpenAI 兼容上游：创建 Channel → 配置 Endpoint 的 `base_url` → 添加 API Key（示例写 `sk-***`）
- Codex OAuth 上游：创建 Channel → 发起 OAuth 授权并导入账号（如遇 `redirect_uri` 回跳端口问题，按后台提示走“复制回调 URL 粘贴完成授权”的兜底流程）

### 4. 配置模型（默认必须）

默认情况下，数据面只允许使用“已启用且已绑定到可用渠道”的模型。你需要：

1) 在管理后台的模型目录（`/admin/models`）添加并启用一个模型（例如 `gpt-4.1-mini`）  
2) 在渠道的模型绑定页（`/admin/channels/{channel_id}/models`）把该模型绑定到你的 Channel（必要时配置 alias/upstream_model）

> 自用模式下如果你只想“原样透传 model”，可以在「系统设置」开启 `feature_disable_models=true` 进入 model passthrough（会关闭 `GET /v1/models`；部分客户端可能依赖该接口）。

### 5. 创建数据面 Token（给客户端用）

登录后在控制台的 `API 令牌` 页面（`/tokens`）创建数据面令牌（`rlm_...`）。令牌明文只在创建/重新生成时展示一次，请妥善保存。

### 6. 用 curl 测试（OpenAI 兼容）

```bash
curl "http://localhost:8080/v1/responses" \
  -H "Authorization: Bearer rlm_..." \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4.1-mini","input":"hello"}'
```

## 2) Web 控制台

功能概览：
- 用户控制台：Token 管理（`/tokens`）、模型列表（`/models`）、订阅/用量（如启用）
- 管理后台（仅 `root`）：上游渠道 / 模型目录 / 系统设置 / 用户管理等

工单（用户支持）功能：
- 用户控制台：`/tickets`（创建/查看自己的工单，支持回复与附件）
- 管理后台（仅 `root`）：`/admin/tickets`（查看全量工单，回复，关闭/恢复）
- 附件：本地存储（`tickets.attachments_dir`），默认保存 7 天后过期清理（`tickets.attachment_ttl`），单次上传附件总大小默认 100MB（`tickets.max_upload_bytes`）

> 界面预览（截图）后续补充。

## 3) 数据面（OpenAI 兼容）

目前支持：
- `POST /v1/responses`
- `GET /v1/models`

认证方式：
- `Authorization: Bearer <token>`（或 `x-api-key`）

### 客户端配置（OpenAI SDK / CLI）

1) 在控制台创建数据面令牌（`rlm_...`）后，配置 OpenAI 环境变量：

Linux/macOS（bash/zsh）：

```bash
export OPENAI_BASE_URL="http://localhost:8080/v1"
export OPENAI_API_KEY="rlm_..."
```

Windows（PowerShell）：

```powershell
# 当前会话
$env:OPENAI_BASE_URL = "http://localhost:8080/v1"
$env:OPENAI_API_KEY = "rlm_..."

# 持久化到用户环境变量（新终端生效）
[System.Environment]::SetEnvironmentVariable("OPENAI_BASE_URL", "http://localhost:8080/v1", "User")
[System.Environment]::SetEnvironmentVariable("OPENAI_API_KEY", "rlm_...", "User")
```

2) （可选）使用 Codex 配置文件（Linux/macOS: `~/.codex/config.toml`；Windows: `%USERPROFILE%\\.codex\\config.toml`）：

```toml
disable_response_storage = true
model_provider = "realms"
model = "gpt-4.1-mini"

[model_providers.realms]
name = "Realms"
base_url = "http://localhost:8080/v1"
wire_api = "responses"
requires_openai_auth = true
```

## 4) 配置要点（建议先看）

### 运行模式

- 自用模式：`self_mode.enable=true`  
  用于个人/小团队自用，禁用订阅/支付/工单等功能域，并让数据面进入 free mode（不校验订阅/余额，仅记录用量）。
- 默认模式：`self_mode.enable=false`  
  面向完整功能（订阅/余额/支付/工单等），配置项更多。

### 反向代理 / TLS 终止

如果部署在 Nginx/Caddy 等反向代理之后，建议显式设置站点地址用于页面展示与回跳链接生成：
- 管理后台「系统设置」中的 `site_base_url`（优先）；或
- 配置文件 `app_settings_defaults.site_base_url`（仅当未被后台覆盖时生效）；或
- `server.public_base_url` 作为兜底

并按需配置 `security.trust_proxy_headers` / `security.trusted_proxy_cidrs` 来控制是否信任 `X-Forwarded-*`。

## 5) 安全说明（必须读）

- 上游 API 密钥 / OAuth 令牌 **明文入库**（BLOB）。
- 用户数据面令牌 / Web 会话 **仅存 hash**（SHA256）。
- `base_url` 会做最小校验（协议/Host/DNS）。

> 注意：历史已加密入库的上游凭证 / OAuth 账号会在迁移中被禁用（需要在管理后台重新录入/重新授权）。

## 6) 开发与测试

开发热重载（自动重启）：

```bash
make dev
```

```bash
go test ./...
```
