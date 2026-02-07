# Realms（OpenAI 风格 API 中转）

Realms 是一个 Go 单体服务（Gin），对外提供 **OpenAI 兼容** 的 API（数据面），并提供一个 Web 控制台（管理面）用于配置上游与下游 Token。

> ✅ 已完成“前后端分离（参考 new-api）”：后端提供 `/api/*` JSON API，并对 `/login` 等页面路径做 SPA fallback；前端工程位于 `web/`（构建产物默认 `web/dist`，也可通过 Docker 在构建期 embed 到二进制）。
>
> 对普通用户/默认部署：推荐“同源一体”（前后端代码分离，但仍由同一台服务器、同一域名/端口提供服务）。详见：`docs/frontend.md`。
>
> 🔐 **对齐 new-api 的防 CSRF 策略**：当使用 cookie session 调用需要登录态的 `/api/*` 接口时，前端会自动附带 `Realms-User: <user_id>` header（跨站请求难以伪造该自定义 header），后端会校验其与会话用户一致。

**你可以用它做什么：**
- 作为 OpenAI SDK / Codex CLI 的 `base_url` 中转层（支持 `POST /v1/responses` SSE 透传）
- 在 Web 控制台里管理用户 Token（`sk_...`）、查看用量与请求明细
- 在管理后台里管理上游渠道（OpenAI 兼容 base_url / Codex OAuth）与路由策略

## 文档

- 在线文档（GitHub Pages）：https://flowerrealm.github.io/realms/
- 环境变量示例：[`.env.example`](.env.example)
- 贡献指南：[`CONTRIBUTING.md`](CONTRIBUTING.md)
- 安全政策：[`SECURITY.md`](SECURITY.md)
- 行为准则：[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)
- 许可证：[`LICENSE`](LICENSE)

## 安装方式（非 Docker）

- **Debian/Ubuntu（.deb）**：从 GitHub Releases 下载 `realms_<TAG>_linux_amd64.deb`（或 `arm64`），安装后默认以 systemd 服务启动（配置：`/etc/realms/realms.env`；数据：`/var/lib/realms`）。
- **Windows（.zip 内含 realms.exe）**：从 GitHub Releases 下载 `realms_<TAG>_windows_amd64.zip`，解压后将 `.env.example` 复制为 `.env` 并运行 `realms.exe`。
- **macOS（.tar.gz）**：从 GitHub Releases 下载 `realms_<TAG>_darwin_arm64.tar.gz`（Apple Silicon）或 `realms_<TAG>_darwin_amd64.tar.gz`（Intel），解压后将 `realms` 放到 PATH（例如 `/usr/local/bin`）并运行。

更完整的部署命令见：`docs/USAGE.md`（Docker Compose 依然是推荐方式）。

## 1) 快速开始（本地）

### 前置

- Go 1.22+
- SQLite（默认，无需额外依赖）或 MySQL 8.x（可选）

### 1. 数据库（SQLite 默认 / MySQL 可选）

#### SQLite（默认，本地/单机部署）

默认使用 SQLite（无需配置文件）。首次启动会自动创建数据库文件并初始化 schema。
如需覆盖 SQLite 数据库路径，可通过环境变量设置：`REALMS_DB_DRIVER=sqlite`、`REALMS_SQLITE_PATH=...`。

#### MySQL（可选，本地开发）

如需使用 MySQL，请在 `.env`（或环境变量）中设置：
- `REALMS_DB_DSN=...`（设置该值即可推断使用 MySQL；也可以显式设置 `REALMS_DB_DRIVER=mysql`）

`make dev` 仅启动本地（正常模式）：`http://127.0.0.1:8080/`（air 热重载）。  
`make dev` 不会自动启动 Docker / MySQL；如你选择使用 MySQL，请先自行启动 MySQL（本机或 docker compose）。
默认会同时启动前端 `web/dist` 的 watch 构建（`npm run build -- --watch`），保证 `/login`、`/admin/*` 等同源页面在开发中实时更新。

如需用 docker compose 启动 MySQL（可选）：

```bash
docker compose up -d mysql
```

> 提示：如果你的机器上 **3306 已被其他 MySQL 占用**，`docker-compose.yml` 的端口映射会冲突。  
> 这时可以：
> 1) 复用现有 MySQL（确保存在 `realms` 数据库）；或  
> 2) 在 `.env` 中设置 `MYSQL_HOST_PORT=13306`（可选 `MYSQL_BIND_IP=127.0.0.1` 仅本机监听），并同步更新 `.env` 的 `REALMS_DB_DSN`（例如 `127.0.0.1:13306`）。

### 2. 启动 Realms

```bash
cp .env.example .env
go run ./cmd/realms
```

> 说明：服务启动会尝试自动加载当前目录的 `.env`（若存在）；也可以通过系统环境变量直接注入配置。编译产物：`./realms` 或容器内 `/realms`。

首次启动会自动执行内置迁移（`internal/store/migrations/*.sql`）。  
当 `db.driver=mysql`：
- 在 `env=dev` 且账号具备权限时，如果目标数据库不存在，会自动创建数据库后继续迁移
- 如果 MySQL 处于启动过程中（常见于刚 `docker compose up`），dev 环境会等待 MySQL 就绪（最多 30s）后再继续

当 `db.driver=sqlite`（默认）：
- 首次启动会执行一次性 schema 初始化（创建全量表/索引）

### 2.1 启动前端（可选）

开发模式（两种）：

1) 同源联调（推荐，后端路由/鉴权更贴近生产）：

```bash
make dev
```

访问：`http://127.0.0.1:8080/login`

2) 前端独立 dev server（Vite + proxy 到 8080）：

```bash
cd web
npm install
npm run dev
```

访问：`http://localhost:5173/login`

如需同源部署（由后端提供静态资源），先构建：

```bash
cd web
npm run build
```

构建产物会输出到 `web/dist`，后端默认会从该目录提供静态资源并对 `/login` 等路径回落到 `index.html`。

### 3. 配置上游（必做）

Realms 本身不自带可用上游，启动后请先完成一次上游配置，否则数据面无法转发请求。

> 说明：当前前端已覆盖用户控制台与管理后台的完整功能入口（路由保持与 SSR/tag `0.3.3` 一致）：
> - 用户：`/login`、`/register`、`/dashboard`、`/announcements`、`/tokens`、`/models`、`/usage`、`/account`、`/subscription`、`/topup`、`/pay/...`、`/tickets`
> - 管理：`/admin/channels`、`/admin/channel-groups`、`/admin/models`、`/admin/users`、`/admin/subscriptions`、`/admin/orders`、`/admin/payment-channels`、`/admin/usage`、`/admin/tickets`、`/admin/announcements`、`/admin/oauth-apps`、`/admin/settings`
在管理后台配置上游：
- OpenAI 兼容上游：创建 Channel → 配置 Endpoint 的 `base_url` → 添加 API Key（示例写 `sk-***`）
- Codex OAuth 上游：创建 Channel → 手动完成 OAuth 授权 → 复制浏览器回调 URL（含 `code/state`）并粘贴完成导入账号

### 4. 配置模型（默认必须）

默认情况下，数据面只允许使用“已启用且已绑定到可用渠道”的模型。你需要：

1) 在管理后台的模型目录（`/admin/models`）添加并启用一个模型（默认推荐 `gpt-5.2`）  
2) 在渠道的模型绑定页（`/admin/channels/{channel_id}/models`）把该模型绑定到你的 Channel（必要时配置 alias/upstream_model）

> 文档中的默认示例模型统一为 `gpt-5.2`。

> 自用模式下如果你只想“原样透传 model”，可以在「系统设置」开启 `feature_disable_models=true` 进入 model passthrough（会关闭 `GET /v1/models`；部分客户端可能依赖该接口）。

### 5. 创建数据面 Token（给客户端用）

登录后在控制台的 `API 令牌` 页面（`/tokens`）创建数据面令牌（`sk_...`）。令牌默认隐藏，可在列表页查看/复制；撤销后无法查看。升级前创建的旧令牌可能无法显示明文，需要重新生成后才能查看。

### 6. 用 curl 测试（OpenAI 兼容）

```bash
curl "http://localhost:8080/v1/responses" \
  -H "Authorization: Bearer sk_..." \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.2","input":"hello"}'
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
- `POST /v1/chat/completions`
- `POST /v1/messages`
- `GET /v1/models`
- `GET /v1beta/models`
- `GET /v1beta/openai/models`
- `POST /v1beta/models/{path...}`

认证方式：
- `Authorization: Bearer <token>`（或 `x-api-key`）

### 客户端配置（OpenAI SDK / CLI）

1) 在控制台创建数据面令牌（`sk_...`）后，配置 OpenAI 环境变量：

Linux/macOS（bash/zsh）：

```bash
export OPENAI_BASE_URL="http://localhost:8080/v1"
export OPENAI_API_KEY="sk_..."
```

Windows（PowerShell）：

```powershell
# 当前会话
$env:OPENAI_BASE_URL = "http://localhost:8080/v1"
$env:OPENAI_API_KEY = "sk_..."

# 持久化到用户环境变量（新终端生效）
[System.Environment]::SetEnvironmentVariable("OPENAI_BASE_URL", "http://localhost:8080/v1", "User")
[System.Environment]::SetEnvironmentVariable("OPENAI_API_KEY", "sk_...", "User")
```

2) （可选）使用 Codex 配置文件（Linux/macOS: `~/.codex/config.toml`；Windows: `%USERPROFILE%\\.codex\\config.toml`）：

```toml
disable_response_storage = true
model_provider = "realms"
model = "gpt-5.2"

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

开发热重载（见上文 2.1，同源联调推荐）：

```bash
make dev
```

```bash
go test ./...
```

### CI（GitHub Actions）

本仓库包含一个会在每次 push 时触发的 CI（见 `.github/workflows/ci.yml`）：
- 单测：`go test ./...`
- E2E：Codex CLI → Realms → 上游（需要配置 GitHub Secrets），用于验证真实链路与用量统计落库

需要在仓库 Secrets 中配置（占位名，勿提交真实密钥到仓库）：
- `REALMS_CI_UPSTREAM_BASE_URL`：上游 OpenAI 兼容 `base_url`（例如 `https://api.openai.com` 或 `https://api.openai.com/v1`）
- `REALMS_CI_UPSTREAM_API_KEY`：上游 API Key（例如 `sk-***`）
- `REALMS_CI_MODEL`：用于 E2E 的模型名（例如 `gpt-5.2`）

> 说明：E2E 同时包含一个“fake upstream”的用例，用于更稳定地覆盖 `cached_tokens` 的解析与落库；真实上游用例也会执行两次请求并要求第二次命中缓存（`cached_input_tokens > 0`）。

在本地复现 E2E（可选）：

```bash
npm install -g @openai/codex
export REALMS_CI_UPSTREAM_BASE_URL="https://api.openai.com"
export REALMS_CI_UPSTREAM_API_KEY="sk-***"
export REALMS_CI_MODEL="gpt-5.2"
go test ./tests/e2e -run TestCodexCLI_E2E -count=1
```

## 7) 版本号

- 运行时构建信息（公开）：
  - 健康检查（含版本/DB 状态）：`GET /healthz`
- release 构建建议通过 `-ldflags -X` 注入版本信息（Docker 发布链路已支持 `REALMS_VERSION/REALMS_BUILD_DATE`）。
- 最新发布版本（latest）由 GitHub Pages 提供（`version.json` / `version.txt`），用于外部查询与升级提示（见 `.github/workflows/pages.yml`）。
- Web 控制台与管理后台默认不在页脚展示版本信息（如需排障，请使用 `/healthz`）。
