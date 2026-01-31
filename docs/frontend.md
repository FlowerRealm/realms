# 前后端分离（默认同源部署）

Realms 采用“前后端分离”的代码结构：

- **后端（Go / Gin）**：提供数据面 `/v1/*`、`/v1beta/*`，以及管理面 JSON API `/api/*`
- **前端（Vite + React）**：位于 `web/`，负责页面路由（`/login`、`/dashboard`、`/admin/*` 等），通过 `/api/*` 调用后端

但对**普通用户/默认部署**来说，Realms 推荐并默认采用 **同源一体** 的运行方式：前后端仍然在**同一台服务器、同一域名/端口**上提供服务（只是在代码层面拆分）。

这样做的好处是：
- 免去跨域 Cookie、第三方 Cookie 限制带来的登录问题
- 只需要部署一个后端进程（或一个镜像），运维更简单

---

## 方式 A（默认推荐）：同源一体（单域/单端口）

在该模式下：
- 后端提供 `/api/*`、`/v1/*` 等接口
- 同时也会提供前端静态资源，并对 `/login` 等页面路径做 SPA 回落（fallback 到 `index.html`）

### A1) Docker（推荐）

默认镜像（**前后端绑在一起**）：`flowerrealm/realms`  
在构建期会先构建前端（`web/dist`），再通过 `-tags embed_web` 将其 embed 到后端二进制中；运行时只需要启动后端容器即可（同源）。

如果你希望“前后端分离部署”（外置前端/独立站点），请使用 **后端专用镜像**：
- `flowerrealm/realms:backend`（跟随最新 tag）
- `flowerrealm/realms:<TAG>-backend`（固定版本）

### A2) 非 Docker（本地/裸机）

1) 构建前端：

```bash
cd web
npm install
npm run build
```

2) 启动后端（默认会从 `./web/dist` 提供静态资源）：

```bash
cd ..
go run ./cmd/realms
```

3) 访问：
- `http://<host>:<port>/login`

相关环境变量：
- `FRONTEND_DIST_DIR`：静态资源目录（默认 `./web/dist`）
- `FRONTEND_BASE_URL`：留空（表示同源提供前端）

---

## 方式 B（可选）：外置前端（仍保持后端为 API）

如果你希望把前端静态资源部署到独立站点（例如 Nginx / CDN），可以：

1) 构建前端产物 `web/dist` 并部署到某个站点（示例：`https://fe.example.com`）
2) 在后端设置：

```bash
FRONTEND_BASE_URL="https://fe.example.com"
```

行为说明：
- 后端遇到“非 API 且未命中路由”的请求时，会 **301 重定向** 到 `FRONTEND_BASE_URL + 原始路径`
- `/api/*`、`/v1/*` 等已注册的路由不受影响（仍由后端处理）

建议：
- 若你使用 Docker 部署后端：优先选择 `flowerrealm/realms:backend`，减少镜像体积（不包含 embed 的前端产物）。

⚠️ 注意：如果前端与后端跨域，浏览器 Cookie/凭证策略会显著复杂（SameSite/CORS/第三方 Cookie 限制）。Realms 默认不推荐跨域部署；更推荐用反向代理把两者放到同域名下（同源）。

---

## 开发模式：前端 dev server + 后端（推荐）

1) 启动后端（默认 8080）：

```bash
go run ./cmd/realms
```

2) 启动前端（默认 5173，已配置 proxy 到 8080）：

```bash
cd web
npm install
npm run dev
```

访问：`http://localhost:5173/login`

---

## 安全提示：Session API 需要 `Realms-User` 请求头

Realms 的部分 cookie session JSON API（例如 `/api/token`、`/api/channel`）要求请求携带：

```
Realms-User: <user_id>
```

并且后端会校验该值与当前会话用户一致，用于降低 CSRF 风险。

- 使用仓库内置前端时：已自动处理（登录后写入 localStorage，并在 API 请求时自动附带）
- 如果你自研前端或直接用浏览器调用这些 API：需要自行附带该 header
