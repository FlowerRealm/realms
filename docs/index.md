# Realms 文档

Realms 是一个 Go 单体服务（`net/http`），对外提供 **OpenAI 兼容** 的 API（数据面），并提供一个 Web 控制台（管理面）用于配置上游与下游 Token。

## 你可以用它做什么

- 作为 OpenAI SDK / Codex CLI 的 `base_url` 中转层（支持 `POST /v1/responses` SSE 透传）
- Web 控制台管理用户 Token（`rlm_...`）、查看用量与请求明细
- 管理后台管理上游渠道（OpenAI 兼容 base_url / Codex OAuth）与路由策略

## 快速开始（本地）

```bash
cp config.example.yaml config.yaml
go run ./cmd/realms -config config.yaml
```

首次启动会自动执行内置迁移（SQLite）或迁移脚本（MySQL）。启动后：

1) 打开 Web 控制台：`http://localhost:8080/`  
2) 注册并登录（开发期默认允许注册：`security.allow_open_registration=true`）  
3) **第一个注册的用户会被设置为 `root`**  
4) 进入管理后台：`http://localhost:8080/admin`

## 常用链接

- 部署：见「部署指南」
- 数据面接口：见「API 手册」
- 健康检查：`GET /healthz`
- 构建信息：`GET /api/version`

