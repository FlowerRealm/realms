# 上游渠道请求体黑白名单

## 背景

不同上游（尤其是 OpenAI 兼容的自定义 base_url/代理）对请求体字段支持不一致，容易出现“渠道可用但请求被 400 拒绝”的情况。

为对齐 new-api 的“按渠道移除字段”思路并提供更通用的兜底能力，Realms 在渠道维度增加请求体黑白名单（基于 JSON path）用于在转发前按渠道过滤字段。

## 字段（upstream_channels）

- `request_body_whitelist`：请求体白名单（JSON 数组，元素为 JSON path）
- `request_body_blacklist`：请求体黑名单（JSON 数组，元素为 JSON path）

> 为空或 `[]` 表示禁用。

## 语义与顺序

在每次选择到具体上游渠道（selection）后，转发前按以下顺序处理：

1. 若配置了 `request_body_whitelist`：仅保留白名单中存在的字段（不存在的 path 会被忽略）
2. 若配置了 `request_body_blacklist`：删除黑名单指定字段
3. 最后执行 `param_override`（管理员可重新设置/补回被过滤字段，符合“管理员改写优先”语义）

## 生效位置

- 数据面 `/v1/responses` 与 `/v1/messages` 均支持
- failover 到另一个渠道时，会重新按“新渠道配置”应用黑白名单（无跨渠道串扰）

## 管理后台

- 在 `/admin/channels` 的渠道行点击“设置”，在“请求体黑白名单”折叠项中配置：
  - 白名单（仅保留）
  - 黑名单（删除字段）
- Admin Config 导出/导入版本为 `5`，导入兼容 `1/2/3/4/5`（`v5` 起包含上述字段）
