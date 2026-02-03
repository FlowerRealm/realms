# 变更提案: frontend-backend-separation

## 元信息
```yaml
类型: 重构
方案类型: implementation
优先级: P1
状态: 草稿
创建: 2026-01-29
参考: QuantumNous/new-api（本机样例: /tmp/new-api）
```

---

## 1. 背景与问题

Realms 当前是 Go 单体（`net/http`），同时承载：
- 数据面（OpenAI 兼容）：`/v1/*`、`/v1beta/*`
- 管理面（Web 控制台 + 管理后台）：服务端渲染（SSR），实现位于 `internal/web/*` 与 `internal/admin/*`，路由集中在 `internal/server/app.go`

这会带来两个直接问题：
1) **前后端耦合**：UI 与业务逻辑（鉴权/Feature Gate/Store）在同一进程同一模块，难以独立迭代与复用。
2) **交付形态受限**：无法像 `new-api/web` 那样用独立前端工程（Vite/React）实现更强的交互、状态管理与构建链路。

---

## 2. 目标

### 2.1 总目标
参考 `new-api` 的组织方式，将 Realms 改造为**前后端分离的 Monorepo**：
- 后端：只负责 API、鉴权、业务规则与数据面转发（仍为 Go）
- 前端：独立工程（建议 React + Vite），通过 `/api/*` 调用后端

### 2.2 约束与保持项
- **数据面兼容性**：保持 `/v1/*`、`/v1beta/*` 行为不变（尤其 `POST /v1/responses` SSE 透传）。
- **同源优先**：默认推荐同源部署（同域名下静态资源 + API 反代），避免跨域 cookie/第三方 cookie 限制带来的登录问题。
- **可选单容器交付**：提供与 `new-api` 类似的“后端 embed `web/dist`”方案，便于一体化部署；同时保留“前端静态部署 + 反代 API”的部署方式。

### 2.3 已确认决策（对齐 new-api）
- **页面路由保持 `/login` 这类路径**：页面由前端 SPA 路由负责；后端仅负责返回 `index.html`（fallback），不再用 SSR 渲染页面。
- **后端 JSON API 使用 `/api/*`**：前端通过 `/api/*` 调用后端（与 `new-api/web` 的调用方式一致）。
- **后端框架对齐为 Gin**：将 Realms 从 `net/http + ServeMux` 迁移到 `gin.Engine`，并采用 `router.SetRouter()` 的分层组织方式（对齐 `new-api/router/*`）。
- **会话机制对齐为 cookie session**：采用 `gin-contrib/sessions` + cookie store（对齐 `new-api/main.go`），默认通过 `SESSION_SECRET` 提供签名密钥。
- **目录结构对齐为仓库根 router/controller**：新增仓库根 `router/`（以及按需新增 `controller/`、`middleware/` 等），不再将 Web/API 路由组装集中放在 `internal/server/app.go`。

---

## 3. 方案

### 3.1 仓库结构（建议）
```text
Realms/
  cmd/realms/                 # 后端入口
  internal/                   # 后端实现
  web/                        # 前端工程（新增，参考 /tmp/new-api/web）
    package.json
    vite.config.js
    src/
    dist/                     # 构建产物（不提交）
```

### 3.2 路由与边界

#### 后端（Go）
- 保留数据面与现有公共接口：
  - `GET /healthz`
  - `GET /api/version`
  - `POST /v1/responses` 等数据面
- 新增 UI 所需 JSON API（建议统一走 `/api/*`）：
  - 认证：`/api/user/login`、`/api/user/logout`、`/api/user/register`、`/api/user/self`
  - Console：`/api/console/*`（tokens/models/usage/announcements/tickets/billing 等）
  - Admin：`/api/admin/*`（channels/channel-groups/models/settings/users/tickets 等）
- UI 路由（如 `/login`、`/dashboard`、`/admin/*`）由前端 SPA 接管：
  - **embed 模式**：后端对非 `/api|/v1|/v1beta|/oauth|/healthz|/assets` 的 GET/HEAD 返回 `index.html`（SPA fallback）
  - **静态站点模式**：Nginx/Caddy 负责 SPA fallback，并反代 `/api|/v1|...` 到后端

> 对齐依据：`/tmp/new-api/router/web-router.go` 的 `router.NoRoute`：当请求前缀为 `/v1`、`/api`、`/assets` 时走 API NotFound；否则回落到 `index.html`。
>
> Realms 当前实现补充：
> - `FRONTEND_BASE_URL`：非 API 路径 NoRoute 直接 301 跳转到外置前端（对齐 new-api）
> - `FRONTEND_DIST_DIR`：本地静态资源目录（默认 `./web/dist`），用于同源部署（embed 模式后续补齐）
> - `SESSION_SECRET`：cookie session 签名密钥（为空时运行期随机生成，重启会导致会话失效）

#### 前端（Vite/React）
- 默认同源：API baseURL 为空字符串（与 `new-api/web/src/helpers/api.js` 一致）
- 开发期通过 Vite proxy 把 `/api`、`/v1` 等转发到 `http://localhost:8080`

### 3.3 鉴权与安全
- 继续使用现有 cookie 会话（`realms_session` / `realms_session_self`），确保同源下的登录体验与安全性。
- 写操作防 CSRF（对齐 new-api）：
  - 采用 `Realms-User: <user_id>` 自定义 header（前端从 localStorage 中读取登录用户并附带），后端校验其与会话用户一致。
  - 说明：跨站表单/图片等传统 CSRF 手段无法附带该自定义 header，从而达到与 new-api 相同的防护效果。
  - 备注：若未来需要跨域部署，再补充 CORS + SameSite=None + Secure 的完整策略（此项默认不做）。

### 3.4（新增）接口映射（MVP）

> 目标：先覆盖“登录 + Token + Admin 基础闭环”，其余功能按模块逐步补齐。

#### 页面路由（前端 SPA）
- 用户侧：`/login`、`/register`、`/dashboard`、`/tokens`、`/models`、`/usage`
- 管理侧：`/admin`、`/admin/channels`、`/admin/models`（以及后续扩展）

#### JSON API（后端）
- 认证（Session + Realms-User）：
  - `POST /api/user/login`
  - `POST /api/user/logout`
  - `POST /api/user/register`（若开放注册）
  - `GET /api/user/self`（返回 user + role + feature 状态）
- Console（用户侧）：
  - `GET /api/token`、`POST /api/token`、`POST /api/token/{id}/rotate`、`POST /api/token/{id}/revoke`、`DELETE /api/token/{id}`
  - 用量：优先复用现有 `GET /api/usage/windows`、`GET /api/usage/events`
- Admin（root）：
  - `GET /api/channel`、`POST /api/channel`
  - `GET /api/models/`、`POST /api/models/`
  - 渠道模型绑定：`POST /api/channel/{channel_id}/models`（以及对应 CRUD）

### 3.5 构建与运行（建议）
- 开发：
  - 后端：`make dev`（或 `go run ./cmd/realms`）
  - 前端：`cd web && npm install && npm run dev`（Vite proxy 到 8080）
- 生产：
  - embed：前端 `npm run build` → 后端 `go build -tags embed_web` embed `web/dist`
  - 静态：前端产物部署到 Nginx；Nginx 反代后端 API

---

## 4. 验收标准

- [ ] `web/` 可独立开发运行：Vite 启动后可通过代理调用后端 `/api/*` 与 `/v1/*`
- [ ] 登录后可完成 Console 最小闭环：创建/轮换/撤销 Token，并用 Token 成功调用 `POST /v1/responses`
- [ ] 管理后台可完成 Admin 最小闭环：创建 Channel、添加/启用 Model、完成渠道模型绑定
- [ ] embed 模式下：访问 `/login`、`/dashboard`、`/admin` 等 UI 路由返回 SPA（非 SSR）
- [ ] `go test ./...` 通过；前端 `npm run build` 通过
