# 架构

> 本页基于 `helloagents/wiki/arch.md` 摘要整理，后续会持续补充与精炼。

## 组成

- **数据面（API）**：对外提供 OpenAI 兼容 API，完成鉴权、调度与上游转发
- **Web 控制台**：用户侧界面（Token/用量/订阅等）
- **管理后台**：root 管理上游渠道、模型、系统设置等
- **Store（DB）**：SQLite（单机默认）或 MySQL（可选、多实例）

## 关键路径（简述）

1) 下游请求进入数据面 handler
2) 完成鉴权与配额/计费逻辑（默认模式或 self_mode/free mode）
3) 选择可用上游（支持 failover）
4) 转发请求到上游并进行最小必要的字段处理
5) 记录用量事件（`usage_events`）用于统计与排障

