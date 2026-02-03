# 轻量迭代：请求明细展示清理（client_disconnect / 仅已完成）

## 目标

- 请求明细不展示 `client_disconnect`（避免用户误解为服务端故障）。
- 请求明细只展示已完成请求，不展示进行中的请求（预留中 / reserved）。

## 任务清单

- [√] 过滤请求明细列表：仅返回非 `reserved` 状态的 usage_events
- [√] 请求明细错误展示：不输出 `client_disconnect`
- [√] 用户 API `/api/usage/events`：同样隐藏 `client_disconnect`，且只返回已完成请求
- [√] 更新知识库：同步 API 行为变更说明
- [√] 运行测试：`go test ./...`
