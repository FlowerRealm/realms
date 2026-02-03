# 上游渠道请求字段策略

## 背景

为对齐 new-api 的“按渠道移除/允许透传字段”能力，Realms 在渠道维度引入请求字段策略，用于控制某些敏感/计费相关字段是否允许透传到上游。

## 策略字段（upstream_channels）

- `allow_service_tier`：允许透传 `service_tier`（默认 `false`）
- `disable_store`：禁用透传 `store`（默认 `false`）
- `allow_safety_identifier`：允许透传 `safety_identifier`（默认 `false`）

## 生效位置

- 数据面 `/v1/responses` 与 `/v1/messages` 在每次选择到具体上游渠道（selection）后，转发前应用该渠道策略
- failover 到另一个渠道时会重新按新渠道策略处理
- 与 `param_override` 的顺序：先执行请求字段策略，再执行 `param_override`（管理员可在改写中重新设置被过滤的字段，对齐 new-api 行为）

## 管理后台

- 在 `/admin/channels` 的渠道行点击“设置”，在“请求字段策略”折叠项中配置上述 3 个开关
- 导出/导入（Admin Config）版本为 `5`，导入兼容 `1/2/3/4/5`（`v2` 起包含上述字段）
