# 上游渠道 new-api 对齐设置项

## 目标

为便于从 `QuantumNous/new-api` 迁移渠道配置，Realms 在 `upstream_channels` 维度补齐 new-api 的若干渠道属性字段与 `setting` JSON（键名对齐）。

> 说明：Realms 仍保持自身的三层结构（channel/endpoint/credential + channel_models 绑定模型），并非完整复刻 new-api 的 channel type / 模型列表 UI。

---

## 渠道属性（meta，对齐 new-api 字段）

存储位置：`upstream_channels` 表列。

- `openai_organization`：OpenAI 组织 ID（注入上游请求头 `OpenAI-Organization`）
- `test_model`：管理后台“测试连接”默认模型（优先级高于模型绑定与默认值）
- `tag`：标记/检索用途（仅保存）
- `remark`：管理后台备注（仅保存）
- `weight`：对齐 new-api 字段（当前 Realms 调度不使用该值）
- `auto_ban`：是否允许调度器在 retriable 失败时封禁渠道（关闭后：credential 冷却仍生效，但不会 ban channel）

---

## 渠道额外设置（setting JSON，对齐 new-api ChannelSettings）

存储位置：`upstream_channels.setting`（JSON）。

- `proxy`：按渠道设置上游网络代理  
  - 支持：`http/https/socks5/socks5h`  
  - `direct/none`：显式禁用代理  
  - 留空：使用环境代理（`HTTP_PROXY/HTTPS_PROXY/NO_PROXY`）
- `thinking_to_content`：将流式 `reasoning_content` 转成 `<think>...` 拼接到 `content`
- `pass_through_body_enabled`：该渠道直接透传原始请求体（跳过模型改写/策略/黑白名单/参数改写/系统提示等链路）
- `system_prompt`：统一注入系统提示  
  - `/v1/responses`：注入 `instructions`  
  - `/v1/chat/completions`：注入/修改 system message  
  - `/v1/messages`：注入/修改 `system`
- `system_prompt_override`：当请求已包含 system/instructions 时，是否将 `system_prompt` 追加到最前
- `force_format`：对齐 new-api 字段；当前 Realms 未实现该行为（仅保存）

---

## 管理后台入口

在 `/admin/channels` 的渠道行点击“设置”，使用：
- “渠道属性（new-api 对齐）”分区（hash：`#meta`）
- “渠道额外设置（new-api setting）”分区（hash：`#setting`）

---

## 迁移/兼容性

- MySQL：迁移 `internal/store/migrations/0048_upstream_channels_newapi_settings.sql`
- SQLite：
  - 基础 schema：`internal/store/schema_sqlite.sql`
  - 启动期自动补列：`internal/store/sqlite_upstream_channels_newapi_settings.go`
- Admin Config 导出/导入版本：`6`（导入兼容 `1/2/3/4/5/6`）

