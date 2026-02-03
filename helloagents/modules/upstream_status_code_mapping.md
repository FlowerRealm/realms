# 上游渠道状态码映射（status_code_mapping）

## 背景

为对齐 new-api 的 `status_code_mapping` 能力，Realms 支持在渠道维度把“上游返回的 HTTP status code”映射为另一个 status code 对外返回，用于兼容部分客户端/网关对非 200 的处理方式。

## 配置字段（upstream_channels）

- `status_code_mapping`：JSON 对象（TEXT），例如：`{"400":"200","429":"200"}`
  - key/value 都要求为**数字字符串**

## 生效位置与说明

- 在 `internal/api/openai/handler.go` 返回下游时生效：仅改写对外 `WriteHeader` 的状态码
- 仅作用于“最终直接返回给下游”的非 2xx 分支，不影响 failover 判定、调度上报与用量/审计口径
- 响应体不做改写（仅变更 HTTP status code）

## 管理后台

- 在 `/admin/channels` 的渠道行点击“设置”，在“状态码映射”折叠项中配置（JSON textarea）
- 导出/导入（Admin Config）版本为 `5`，导入兼容 `1/2/3/4/5`
