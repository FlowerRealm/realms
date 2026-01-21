# Realms（统一中转服务）

本仓库包含一个 **OpenAI 风格 API 中转层**：`realms`（Go 单体服务，`net/http`），以及对应的知识库（`helloagents/`）。

## 1) 快速开始（本地）

### 前置

- Go 1.22+
- MySQL 8.x（或兼容）

### MySQL（本地开发）

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

### 启动 realms

```bash
cp config.example.yaml config.yaml
go run ./cmd/realms -config config.yaml
```

### 开发热重载（自动重启）

仅用于开发环境：监听文件变更，自动重新编译并重启进程（不包含浏览器自动刷新）。

```bash
make dev
```

> 注意：移除应用层加密后，历史已加密入库的上游凭证 / OAuth 账号会在迁移中被禁用（需要在管理后台重新录入/重新授权）。

首次启动会自动执行内置迁移（`internal/store/migrations/*.sql`）。  
在 `env=dev` 且账号具备权限时，如果目标数据库不存在，会自动创建数据库后继续迁移。  
如果 MySQL 处于启动过程中（常见于刚 `docker compose up`），dev 环境会等待 MySQL 就绪（最多 30s）后再继续。

## 2) Web 控制台

- 访问：`http://localhost:8080/`
- 默认允许注册（开发期）：`security.allow_open_registration=true`
- **第一个注册的用户会被设置为 `root`**

登录后可在控制台的 `API 令牌` 页面管理数据面令牌（创建/重新生成时展示一次，可随时重新生成）。

工单（用户支持）功能：
- 用户控制台：`/tickets`（创建/查看自己的工单，支持回复与附件）
- 管理后台（仅 `root`）：`/admin/tickets`（查看全量工单，回复，关闭/恢复）
- 附件：本地存储（`tickets.attachments_dir`），默认保存 7 天后过期清理（`tickets.attachment_ttl`），单次上传附件总大小默认 100MB（`tickets.max_upload_bytes`）

管理后台：
- `http://localhost:8080/admin`（仅 `root`）
- 可配置上游：渠道 / 端点 / 凭证（API 密钥明文入库）
- 用户管理：启用/禁用/删除（硬删除会清理令牌/会话/用量/审计等关联数据）
- 启动期配置：修改启动参数 `-config` 指定的 YAML 后重启服务生效（多实例部署请使用共享卷或配置分发）

## 3) 数据面（OpenAI 兼容）

目前支持：
- `POST /v1/responses`
- `GET /v1/models`

认证方式：
- `Authorization: Bearer <token>`（或 `x-api-key`）

示例：

（推荐）Codex CLI 配置模板（跨平台）：

1) 在 Web 控制台创建数据面令牌（`rlm_...`），并配置 Codex 的 OpenAI 环境变量：

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

## 4) 安全说明（必须读）

- 上游 API 密钥 / OAuth 令牌 **明文入库**（BLOB）。
- 用户数据面令牌 / Web 会话 **仅存 hash**（SHA256）。
- `base_url` 会做最小校验（协议/Host/DNS）。
- 如部署在反向代理/TLS 终止之后，建议显式设置“站点地址”：管理后台「系统设置」中的 `site_base_url`（优先）；或在配置文件中设置 `app_settings_defaults.site_base_url`（仅当未被后台覆盖时生效），也可用 `server.public_base_url` 作为兜底。用于页面展示、支付回调/返回地址与 Codex OAuth 回跳链接生成。
- `security.trust_proxy_headers` / `security.trusted_proxy_cidrs` 用于控制是否信任 `X-Forwarded-*` 头（默认不信任；仅当请求来自 `trusted_proxy_cidrs` 时才会读取；如需信任所有来源可显式配置 `0.0.0.0/0` 与 `::/0`，不推荐）。

## 5) 测试

```bash
go test ./...
```
