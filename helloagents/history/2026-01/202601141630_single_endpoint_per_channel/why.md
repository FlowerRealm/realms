# 变更提案: 上游渠道收敛为单 Endpoint

## 需求背景

当前上游配置采用三层结构：`Channel → Endpoint → Credential/Account`，其中一个 Channel 允许创建多个 Endpoint（base_url）。

这会带来管理复杂度：

- 管理后台需要先创建 Endpoint，再进入 Endpoint 绑定 Key/账号。
- 为了“多账号/多 Key”，容易被迫创建多个 Endpoint，结构冗余且难维护。
- 调度链路与数据结构被迫长期兼容“多 Endpoint”分支，心智负担重。

## 目标

1. **每个 Channel 固定 1 个 Endpoint**（base_url 作为 Channel 的唯一上游入口配置）。
2. **Codex OAuth 渠道**：在同一 Endpoint 下可绑定多个账号（账号池）。
3. **自定义 URL（openai_compatible）渠道**：在同一 Endpoint 下可绑定多个 Key（Key 池）。

## 影响范围

- **管理后台（SSR）**：Channels/Endpoints 页面的交互收敛，创建流程简化。
- **数据迁移**：需要合并存量的多 Endpoint 数据，并加唯一约束防止回退。
- **调度链路**：保持三层结构不变，但 Endpoint 层语义从“多”收敛为“单”。

