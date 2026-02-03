# 任务清单（标准开发）

目标：简化上游渠道配置为“每渠道一个 Endpoint”，并在单 Endpoint 下支持多账号/多 Key。

- [√] 数据迁移：合并存量多 Endpoint 数据并加唯一约束（每 channel 仅 1 条 endpoint）
- [√] Store：补齐按 channel 获取/设置 Endpoint 的方法
- [√] Admin UI：Channels 展示 Base URL、创建 Channel 同时配置 base_url、Endpoint 页收敛为单配置页
- [√] 兼容处理：保留旧路由但禁用删除 Endpoint；创建 Endpoint 语义改为 upsert base_url
- [√] 知识库同步：更新 `wiki/api.md`、`wiki/data.md`、`wiki/arch.md`、`wiki/modules/realms.md`、`CHANGELOG.md`
- [√] 测试验证：`go test ./...`
- [√] 更新历史索引：`helloagents/history/index.md`

