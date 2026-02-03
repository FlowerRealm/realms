# 渠道参数改写（param_override，new-api 对齐）

## 目标

在 Realms 中对齐 new-api 的 `param_override`：为每个上游渠道（`upstream_channels`）配置一段 JSON（new-api `operations` 兼容），用于在转发前对请求体执行可控的 JSON 路径改写。

## 范围

- 数据库：为 `upstream_channels` 增加 `param_override`（TEXT，JSON 对象）
- 调度：将 `param_override` 随 `scheduler.Selection` 带到数据面
- 数据面：
  - `/v1/responses` 与 `/v1/messages` 在**每次 selection** 转发前应用 `param_override`
  - failover 到其他渠道时按新渠道重新应用（避免串扰）
  - 顺序：模型 alias rewrite → `param_override` → 请求字段策略（`service_tier/store/safety_identifier`）
- 管理后台：渠道页新增“参数改写（param_override）”编辑弹窗与保存接口
- 导出/导入：导出版本升级为 `3`，导入兼容 `1/2/3`

## 配置格式（与 new-api 一致）

- `param_override` 为 JSON 对象
  - 推荐：`{"operations":[{"path":"...","mode":"set","value":...,"conditions":[...]}]}`
  - 兼容：legacy 顶层 key 覆盖（未提供 `operations` 时）

## 风险与约束

- `param_override` 属于管理员配置能力，错误规则可能导致请求体改写失败从而返回 500；需在配置时确保规则可用。
- 通过“请求字段策略”兜底敏感字段（即使 `param_override` 设置了相关字段，仍会按渠道策略最终过滤）。

## 验收标准

- 管理后台可为渠道编辑并保存 `param_override`（非法 JSON 会被拒绝）
- `/v1/responses` 与 `/v1/messages` 转发前按渠道应用 `param_override`，failover 场景下不串扰
- Admin Config 导出/导入包含 `param_override`（版本 `3`），并兼容旧版本导入
- `go test ./...` 通过

