# 实施方案: 上游渠道收敛为单 Endpoint

## 1. 数据迁移与约束

- 新增迁移：`internal/store/migrations/0006_single_endpoint_per_channel.sql`
  - 对每个 `channel_id` 选取主 Endpoint（`priority DESC, id DESC`）。
  - 将其它 Endpoint 的 `openai_compatible_credentials` / `codex_oauth_accounts` 迁移到主 Endpoint。
  - 删除多余 Endpoint。
  - 为缺失 Endpoint 的 Channel 补齐默认值：
    - `codex_oauth`: `https://chatgpt.com/backend-api/codex`
    - `openai_compatible`: `https://api.openai.com`
  - 为 `upstream_endpoints.channel_id` 添加唯一约束，强制 **每 Channel 一条 Endpoint**。

## 2. Store 层 API

- 新增：
  - `GetUpstreamEndpointByChannelID`：按 channel_id 获取主 Endpoint（兼容历史数据时的排序）。
  - `SetUpstreamEndpointBaseURL`：设置/补齐该 Channel 的 Endpoint base_url（不存在则创建）。

## 3. 管理后台（SSR）收敛

- Channels 列表页：
  - 增加 Base URL 列直接展示当前 Channel 的唯一 Endpoint。
  - 提供“配置”入口（进入单 Endpoint 配置页）。
  - 直接提供 Keys/授权入口（同一 Endpoint 下多 Key/多账号）。
- Channel 创建：
  - 创建 `openai_compatible` Channel 时要求填写 base_url，并自动创建其唯一 Endpoint。
- Endpoint 页面：
  - 从“多 Endpoint 列表/创建”收敛为“单 Endpoint 配置（base_url）+ 鉴权入口”。
  - 禁用 Endpoint 删除入口（每个 Channel 必须存在 Endpoint）。

## 4. 兼容性策略

- 保留原有路由与资源结构（Endpoint/credential/account 仍以 endpoint_id 作为归属键）。
- `POST /admin/channels/{channel_id}/endpoints` 语义从“创建 Endpoint”收敛为“设置该 Channel 的唯一 Endpoint base_url（upsert）”。
- Endpoint 删除接口保留但返回错误提示，避免误删导致 Channel 不可用。

