# 项目上下文（Realms）

## 简介

Realms 是一个 Go 单体服务（Gin），对外提供 **OpenAI 兼容** API（数据面），并提供 Web 控制台（管理面）用于配置上游渠道、用户 Token、模型目录、订阅/充值/工单等。

## 目录结构速览

- 后端入口：`cmd/realms/main.go`
- 路由：`router/`（`/api/*` JSON API + SPA fallback）
- 数据与持久化：`internal/store/`（MySQL/SQLite）
- 前端工程：`web/`（Vite + React Router，构建产物默认 `web/dist`）
- Go 测试：`tests/`（含 Codex CLI E2E）

## 常用命令

- 运行后端（开发）：`make dev` 或 `go run ./cmd/realms`
- 构建前端：`npm --prefix web run build`
- Go 测试：`go test ./...`
- Web E2E（Playwright）：`npm --prefix web run test:e2e`

