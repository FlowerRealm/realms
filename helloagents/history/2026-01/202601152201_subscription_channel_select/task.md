# Task：订阅渠道选择（channel_group）

- [√] 数据库：为 `subscription_plans` 增加 `channel_group`（默认 `default`）
- [√] Store：补齐 `SubscriptionPlan.ChannelGroup` 的读写与查询扫描
- [√] 管理后台：订阅套餐新增/编辑加入分组下拉，列表展示分组
- [√] 数据面：Reserve 返回分组并下发到调度约束（按订阅分组筛选渠道）
- [√] 知识库：更新 data/api/changelog
- [√] 验证：`go test ./...`

