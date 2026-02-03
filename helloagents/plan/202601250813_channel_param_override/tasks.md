# 任务清单：渠道参数改写（param_override，new-api 对齐）

- [√] 数据库：新增 `upstream_channels.param_override`（MySQL 迁移 + SQLite schema + SQLite 启动期补齐列）
- [√] Store：读写 `param_override`（List/Get/Update；保存时校验 JSON 对象）
- [√] 调度：在 `scheduler.Selection` 中携带 `param_override`
- [√] 数据面：`/v1/responses` 与 `/v1/messages` 每次 selection 转发前应用 `param_override`（并保证 failover 不串扰）
- [√] 管理后台：渠道页新增“参数改写（param_override）”弹窗与保存接口
- [√] 导出/导入：版本升级到 `3`，导入兼容 `1/2/3`
- [√] 测试：覆盖 failover 场景下的“按渠道 param_override 不串扰”

