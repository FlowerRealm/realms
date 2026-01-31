# Realms Web（Vite + React）

该目录是 Realms 的前端工程（SPA），对齐 new-api 的“页面路由由前端负责，后端仅做 API + SPA fallback”的组织方式。

## 本地开发

1) 启动后端（默认 8080）：

```bash
go run ./cmd/realms
```

2) 启动前端（默认 5173，已配置 proxy 到 8080）：

```bash
npm install
npm run dev
```

访问：`http://localhost:5173/login`

## 构建

```bash
npm run lint
npm run build
```

构建产物输出到 `web/dist`：
- 同源部署：后端会从 `FRONTEND_DIST_DIR`（默认 `./web/dist`）提供静态资源，并对 `/login` 等路径回落到 `index.html`
- Docker：镜像构建期会先 `npm run build`，再用 `-tags embed_web` 将 `web/dist` embed 到二进制

## 环境变量（可选）

- `VITE_API_BASE_URL`：API baseURL（默认空字符串，同源）
