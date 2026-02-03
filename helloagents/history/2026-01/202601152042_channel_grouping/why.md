# 变更提案: 渠道分组（channel grouping）

## 需求背景
参考 new-api 的分组思路，希望在 Realms 中引入“渠道分组”的概念，用于将不同上游渠道划分到不同分组，并让不同用户在数据面调度时只能使用所属分组内的渠道。

这不是“租户”概念：上游资源与定价仍是全局配置，仅新增一个 **调度维度** 来隔离渠道使用范围。

## 变更内容
1. 数据模型：
   - `users.channel_group`：用户所属渠道分组（默认 `default`）。
   - `upstream_channels.groups`：渠道所属分组（默认 `default`；逗号分隔多个分组）。
2. 调度约束：
   - 数据面请求在调度时强制附带 `RequireChannelGroup`，只会在匹配分组的渠道集合中选择。
   - 粘性绑定/会话绑定不会绕过分组约束。
3. 管理后台支持配置：
   - `/admin/users` 用户资料编辑中可配置 `channel_group`。
   - `/admin/channels` 创建渠道时可设置 `groups`；在渠道 Endpoint 页可编辑 `groups`。

## 影响范围
- **数据库迁移**：新增列 `users.channel_group`、`upstream_channels.groups`。
- **核心链路**：
  - TokenAuth：需要将用户分组注入到 `Principal`。
  - Scheduler：新增分组约束并纳入绑定匹配逻辑。
  - Admin UI：增加字段展示与编辑入口。

## 风险评估
- **风险：分组配置错误导致“无可用渠道”。**
  - **缓解：** 默认值统一为 `default`；管理后台输入做格式校验；保存失败会回显错误。
- **风险：粘性绑定绕过分组。**
  - **缓解：** 将渠道分组写入 `Selection`，并在绑定命中校验 `RequireChannelGroup`。

