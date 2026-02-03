# 任务清单：渠道请求字段策略（new-api 对齐）

- [√] 数据库：新增 `upstream_channels` 字段（MySQL 迁移 + SQLite schema）
- [√] Store：读写渠道策略（List/Get/Update/Create）
- [√] 调度：在 `scheduler.Selection` 中携带渠道策略
- [√] 数据面：`/v1/responses` 与 `/v1/messages` 每次 selection 转发前应用字段策略
- [√] 管理后台：渠道页新增“请求字段策略”弹窗与保存接口
- [√] 导出/导入：版本升级到 `2`，导入兼容 `1/2`
- [√] 测试：覆盖 failover 场景下的“按渠道策略不串扰”

