# 任务清单（轻量迭代）

目标：修复管理后台「渠道列表」闪电按钮测试连接时报“参数错误”。

## 任务

- [√] 定位：确认报错入口为 `POST /admin/channels/*/test`（闪电按钮）
- [√] 修复：测试接口兼容从表单读取 `channel_id`（path 参数异常时兜底）
- [√] 修复：测试表单补齐隐藏字段 `channel_id`
- [√] 兼容：增加 `POST /admin/channels/test` 路由（表单传参）
- [√] 验证：运行 `go test ./...`
- [√] 同步：更新知识库与 Changelog
- [√] 归档：迁移方案包至 `helloagents/history/` 并更新索引
