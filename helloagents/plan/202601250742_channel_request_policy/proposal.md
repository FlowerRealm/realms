# 渠道请求字段策略（new-api 对齐）

## 目标

在 Realms 中对齐 new-api 的“按渠道移除/允许透传字段”能力：对每个上游渠道（upstream_channels）配置请求字段策略，并在数据面转发前生效。

## 范围

- 数据库：为 `upstream_channels` 增加三个布尔开关
  - `allow_service_tier`：是否允许透传 `service_tier`（默认不允许）
  - `disable_store`：是否禁用透传 `store`（默认不禁用）
  - `allow_safety_identifier`：是否允许透传 `safety_identifier`（默认不允许）
- 调度：将渠道策略随 `scheduler.Selection` 一并带到转发链路
- 数据面：
  - `/v1/responses` 与 `/v1/messages` 在**每次选择（selection）**转发前应用策略
  - failover 到其他渠道时，使用目标渠道的策略重新处理（避免“首次尝试删除字段导致后续渠道无法透传”）
- 管理后台：渠道页新增“请求字段策略”配置弹窗
- 导出/导入：导出版本升级为 `2`，导入兼容 `1` 与 `2`

## 默认策略（与 new-api 一致）

- `service_tier`：默认过滤（避免额外计费风险）；仅当渠道显式允许时透传
- `store`：默认允许透传；当渠道显式禁用时移除
- `safety_identifier`：默认过滤（保护用户隐私）；仅当渠道显式允许时透传

## 风险与兼容性

- 行为变化：历史上客户端传入的 `service_tier` / `safety_identifier` 将在默认情况下被过滤；若需要透传必须在目标渠道显式开启。
- Codex OAuth：Codex 请求体有自己的兼容改写逻辑，且会强制/删除部分字段；渠道策略主要用于 OpenAI 兼容与 Anthropic 上游。

## 验收标准

- 管理后台可对每个渠道独立配置三项开关，并能持久化到数据库
- `/v1/responses` 与 `/v1/messages` 转发前按渠道策略过滤字段，且 failover 场景下不会串扰
- `go test ./...` 通过

