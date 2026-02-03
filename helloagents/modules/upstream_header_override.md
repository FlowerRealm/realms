# 上游渠道请求头覆盖（header_override）

## 背景

为对齐 new-api 的 `header_override` 能力，Realms 支持在渠道维度覆写/补充转发到上游的 HTTP 请求头。

## 配置字段（upstream_channels）

- `header_override`：JSON 对象（TEXT），例如：`{"OpenAI-Organization":"org_xxx","X-Proxy-Key":"{api_key}"}`

支持变量替换：
- `{api_key}`：替换为该次请求实际选中的上游凭据（OpenAI/Anthropic 为 API key；Codex OAuth 为 access token）

## 生效位置与顺序

- 在 `internal/upstream/executor.go` 构造上游请求时应用（与业务 JSON 请求体无关）
- 应用顺序：
  - 复制下游可透传 Header（并剥离 Cookie/Host 等敏感与 hop-by-hop header）
  - 清理下游鉴权与压缩语义（`Authorization`/`x-api-key`/`Accept-Encoding`）
  - 应用 `header_override`
  - 注入默认鉴权
    - 因默认鉴权最后注入，`header_override` 无法覆盖 `Authorization`（对齐 new-api 调用顺序）

## 管理后台

- 在 `/admin/channels` 的渠道行点击“设置”，在“Header Override”折叠项中配置（JSON textarea）
- 导出/导入（Admin Config）版本为 `5`，导入兼容 `1/2/3/4/5`
