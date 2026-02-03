# 任务清单（轻量迭代）

目标：修复 Codex OAuth 流式请求在 `request_passthrough=true` 场景下因上游不支持 `/v1/responses` 触发的 `404 Not Found`（管理后台测速误报）。

## 任务

- [√] 修复：Codex OAuth 上游在 `/v1/responses` 返回 404 时，自动兜底重试旧版 `/responses` 路径
- [√] 验证：运行 `go test ./...`
- [√] 同步：更新知识库与 Changelog
- [√] 归档：迁移方案包至 `helloagents/history/` 并更新索引

