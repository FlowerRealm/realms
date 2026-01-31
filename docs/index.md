# Realms 文档

Realms 是一个 Go 单体服务（Gin），对外提供 **OpenAI 兼容** 的 API（数据面），并提供一个 Web 控制台（管理面）用于配置上游与下游 Token。

> ✅ 已完成“前后端分离（参考 new-api）”：后端提供 `/api/*` JSON API，并对 `/login` 等页面路径做 SPA fallback；前端工程位于 `web/`（构建产物默认 `web/dist`，也可在 Docker 构建期 embed 到二进制）。
>
> 默认推荐“同源一体”部署：前后端在代码层面拆分，但对用户来说仍是同一台服务器、同一域名/端口提供服务（避免跨域 Cookie 问题）。详见：[前后端分离](frontend.md)。

## 你可以用它做什么

- 作为 OpenAI SDK / Codex CLI 的 `base_url` 中转层（支持 `POST /v1/responses` SSE 透传）
- Web 控制台管理用户 Token（`rlm_...`）、查看用量与请求明细
- 管理后台管理上游渠道（OpenAI 兼容 base_url / Codex OAuth）与路由策略

## 快速开始（本地）

```bash
cp .env.example .env
go run ./cmd/realms
```

首次启动会自动执行内置迁移（SQLite）或迁移脚本（MySQL）。

## 启动前端（可选）

开发模式（Vite dev server + proxy 到 8080）：

```bash
cd web
npm install
npm run dev
```

访问：`http://localhost:5173/login`

同源部署（由后端提供静态资源）：

```bash
cd web
npm run build
```

## 常用链接

- 部署：见「部署指南」
- 数据面接口：见「API 手册」
- 健康检查：`GET /healthz`
- 构建信息：`GET /api/version`
