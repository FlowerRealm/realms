# 模块索引

> 通过此文件快速定位模块文档

## 模块清单

| 模块 | 职责 | 状态 | 文档 |
|------|------|------|------|
| openai-api | `/v1/*` 北向接口、failover、SSE 透传 | 🚧 | [openai-api.md](./openai-api.md) |
| scheduler | 上游选择与运行态（亲和/粘性/冷却/封禁/探测） | 🚧 | [scheduler.md](./scheduler.md) |
| upstream | 上游 HTTP 请求构造与转发（含 SSE pump） | 🚧 | [upstream.md](./upstream.md) |
| middleware | RequestID/AccessLog/TokenAuth/BodyCache 等数据面中间件 | 🚧 | [middleware.md](./middleware.md) |
| testing | 测试分层、统一入口（Codex/Playwright）与 CI 约定 | 🚧 | [testing.md](./testing.md) |
| web-theme | 前端主题色与 Bootstrap 覆盖点（避免默认亮蓝色） | 🚧 | [web-theme.md](./web-theme.md) |

## 模块依赖关系

```
router/* → middleware → openai-api → scheduler → store/*
                              ↘ upstream → security/*
```

## 状态说明
- ✅ 稳定
- 🚧 开发中
- 📝 规划中
