# 任务清单（轻量迭代）

目标：修复管理后台渠道列表闪电“测试连接”提示“已保存”的误导/误路由问题，并明确测试会写入的内容。

## 任务

- [√] 修复：闪电测试表单统一提交到 `POST /admin/channels/test`（携带 `channel_id`），避免 `/admin/channels/{id}/test` 在部分环境被改写/误匹配
- [√] 验证：运行 `go test ./...`
- [√] 同步：更新 Changelog
- [√] 归档：迁移方案包至 `helloagents/history/` 并更新索引
