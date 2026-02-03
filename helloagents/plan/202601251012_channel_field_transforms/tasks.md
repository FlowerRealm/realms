# 任务清单: channel-field-transforms

- [√] DB：MySQL 迁移新增 3 列（模型后缀保护名单、请求体黑名单、请求体白名单）
- [√] DB：SQLite schema 与启动期补齐列
- [√] Store：UpstreamChannel/查询/更新接口补齐字段
- [√] Scheduler：Selection 携带并注入上述字段
- [√] 数据面：`/v1/responses` 与 `/v1/messages` 增加字段转换与黑白名单处理（每次 selection 转发前）
- [√] 管理后台：`/admin/channels` 增加 3 个配置弹窗与保存接口
- [√] 导出/导入：升级版本并兼容旧版本导入
- [√] 测试：覆盖后缀解析、tokens 字段规范化、黑白名单 per-channel + failover
- [√] 验证：`gofmt` + `go test ./...`
