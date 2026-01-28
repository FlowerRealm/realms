# 数据

> 本页基于 `helloagents/wiki/data.md` 摘要整理，后续会持续补充与精炼。

## 用量事件（usage_events）

Realms 会记录每次数据面请求的用量/状态等信息，主要用途：

- 用量统计（用户侧 / 管理侧）
- 故障排查（失败原因、上游响应摘要等）
- 多实例部署下的统计汇总（基于共享 DB）

## 迁移

- SQLite：首次启动会初始化 schema
- MySQL：启动时应用迁移（`internal/store/migrations/*.sql`）

