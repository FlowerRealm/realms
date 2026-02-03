# 任务清单: frontend-backend-separation

目录: `helloagents/plan/202601292015_frontend-backend-separation/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 44
已完成: 40
完成率: 91%
```

---

## 任务列表

### 1. 方案定稿
- [√] 1.1 确认交付形态：优先 `embed web/dist`（对齐 new-api），并保留“静态站点 + 反代 API”部署方式
- [√] 1.2 确认前端技术栈：React + Vite（参考 `/tmp/new-api/web`；包管理默认 npm）
- [√] 1.3 定义 UI/API 边界：页面路由走 `/login` 等 SPA；后端 JSON API 走 `/api/*`
- [√] 1.4 明确包结构对齐力度：采用仓库根 `router/`（以及按需新增 `controller/`），对齐 new-api

### 2. Backend：Gin 化与路由分层（对齐 new-api）
- [√] 2.1 引入 Gin（`gin-gonic/gin`）
- [√] 2.1.1 引入 cookie session 依赖（`gin-contrib/sessions`）
- [√] 2.1.2 引入静态/压缩依赖（`gin-contrib/static`、`gin-contrib/gzip`）
- [√] 2.2 迁移启动入口：`server.NewApp` → `router.SetRouter(engine, ...)`（对齐 `new-api/router/*`）
- [-] 2.3 迁移基础中间件为 Gin middleware：RequestID/AccessLog/BodyCache/MaxBytes/Timeout（保持现有 net/http middleware 链复用）
- [-] 2.4 迁移鉴权与 feature gate 为 Gin middleware：TokenAuth/SessionAuth/RequireRoles/FeatureGate（保持现有 net/http middleware 链复用）
- [-] 2.5 迁移 CSRF：支持 `X-CSRF-Token` 与 `_csrf`（当前依赖 SameSite=Strict；后续如需跨域再补齐）
- [√] 2.6 数据面路由挂载：`/v1/*`、`/v1beta/*`（确保 SSE 不被 gzip 影响；按 new-api 的中间件注册顺序组织）
- [√] 2.7 会话机制：启用 cookie session（`SESSION_SECRET`）
- [ ] 2.7.1 替换 DB session：移除 `user_sessions` 与 legacy `middleware.SessionAuth`（含 Codex OAuth callback 校验逻辑对齐）
- [√] 2.8 WebRouter：SPA fallback（按 `/v1|/v1beta|/api|/oauth|/assets|/healthz` 分流，参考 `/tmp/new-api/router/web-router.go`）
- [√] 2.8.1 WebRouter：embed `web/dist`（对齐 `/tmp/new-api/main.go` 的 `//go:embed web/dist`）
- [√] 2.9 可选：支持 `FRONTEND_BASE_URL`（类似 new-api，用于外置前端）

### 3. Backend：UI JSON API（/api/*）
- [√] 3.1 统一 JSON 响应与错误结构（success/message/data），避免在 API 下返回 HTML（302/login 或 http.Error）
- [√] 3.2.1 认证：实现 `POST /api/user/login`、`GET /api/user/logout`、`GET /api/user/self`
- [√] 3.2.2 认证：实现 `POST /api/user/register`（按 allow_open_registration 与 email verification）
- [√] 3.3 Console：tokens CRUD（创建/轮换/撤销/删除）
- [√] 3.3.1 Console：tokens 路由对齐：`/api/token`；session user id key 对齐为 `id`
- [√] 3.4 Console：models 列表
- [√] 3.5 Console：usage windows/events（复用现有实现并统一输出结构）
- [√] 3.6 Admin：channels CRUD + 健康探测/测试（最小闭环）
- [√] 3.6.1 Admin：实现 `/api/channel`（root session）最小闭环：列表/创建/查询/删除
- [√] 3.7 Admin：models CRUD + 启用/绑定到渠道
- [-] 3.8 Admin：channel groups（如需要）
- [-] 3.9 Admin：users（如需要）
- [√] 3.10 停用 SSR 路由：`/login`、`/dashboard`、`/admin/*` 不再由后端渲染，改为 SPA

### 4. Frontend：工程初始化
- [√] 4.1 创建 `web/`（Vite + React），目录结构参考 `/tmp/new-api/web`
- [√] 4.2 `vite.config.js` 配置 proxy：`/api`、`/v1`、`/v1beta` → `http://localhost:8080`
- [√] 4.3 封装 API client（axios）：同源默认 + cookie session

### 5. Frontend：路由与页面（MVP）
- [√] 5.1.1 用户侧路由：`/login`、`/dashboard`、`/tokens`（MVP）
- [√] 5.1.2 用户侧路由：`/models`、`/usage`
- [√] 5.2.1 管理侧路由：`/admin`（占位）
- [√] 5.2.2 管理侧路由：`/admin/channels`、`/admin/models`
- [√] 5.2.2.1 管理侧路由：实现 `/admin/channels`（列表/创建/删除）
- [√] 5.2.2.2 管理侧路由：实现 `/admin/models`
- [√] 5.3 登录态与权限守卫：普通用户 vs root
- [√] 5.4 冒烟闭环：创建数据面 Token → curl `POST /v1/responses` 成功（E2E 覆盖）

### 6. 构建与部署（对齐 new-api）
- [√] 6.1 Dockerfile：加入 npm build stage（参考 `/tmp/new-api/Dockerfile`），并将 `web/dist` embed 到镜像内二进制
- [-] 6.2 Makefile：增加 `make web-dev` / `make web-build`（可选）
- [-] 6.3 docker-compose：同源部署样例（可选；外置前端可用 `FRONTEND_BASE_URL`）

### 7. 测试与验收
- [√] 7.1 后端：为 gin router 与鉴权/Realms-User 增补单测
  - 备注：对齐 new-api 的策略，但 header 名使用 `Realms-User`。
- [√] 7.2 回归：`POST /v1/responses` SSE（不被 gzip/缓冲破坏）
- [√] 7.3 运行 `go test ./...`
- [√] 7.4 前端：`npm run build` 通过（可选增加 lint）
