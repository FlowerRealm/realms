# 上游渠道参数改写（param_override）

## 背景

为对齐 new-api 的 `param_override` 能力，Realms 在渠道维度引入请求体参数改写：允许管理员按渠道配置一组 JSON 路径操作，用于在转发到上游前对请求体做确定性改写。

## 配置字段（upstream_channels）

- `param_override`：JSON 对象（TEXT），内容为 new-api 兼容格式（支持 `operations` 列表；也兼容 legacy 的“顶层键覆盖”）。

## 生效位置与顺序

- 数据面 `/v1/responses` 与 `/v1/messages` 在**每次选择到具体上游渠道（selection）**后，转发前应用该渠道的 `param_override`
- 应用顺序：模型 alias rewrite →（Responses 模型后缀解析）→ 请求字段策略（`service_tier/store/safety_identifier`）→ 请求体黑白名单 → `param_override`
- 说明：请求字段策略/黑白名单用于默认过滤“用户透传字段”；`param_override` 作为管理员改写可以重新设置这些字段（对齐 new-api 行为）
- failover 到另一个渠道时会重新按新渠道的 `param_override` 处理

## 管理后台

- 在 `/admin/channels` 的渠道行点击“设置”，在“参数改写（param_override）”折叠项中配置（JSON textarea）
- 导出/导入（Admin Config）版本为 `5`，导入兼容 `1/2/3/4/5`
