# 技术方案：移除 Web 对话功能（/chat）

## 方案概述

采用“直接删除”的路径，而不是保留 build tag（`no_webchat`）或运行期开关：
- 删除 Web `/chat` 页面与 `/api/chat/*` 接口实现。
- 下线数据面 `POST /v1/chat/completions`，统一以 `POST /v1/responses` 作为唯一数据面入口。
- 删除与 Web chat 强耦合的“对话分组（X-Realms-Chat）”能力。
- 删除与 Web chat 强耦合的 SearXNG 搜索覆盖能力（保留未来需要时可重新引入，但本轮不做兼容层）。
- 通过新增 SQL 迁移，**清理数据库中残留的 chat 配置项与对话 Token**。

## 代码改动点

### 1) 路由与 Web 模板
- `internal/server/`：移除 Web chat 路由注册逻辑（不再注册 `/chat` 与 `/api/chat/*`）。
- `internal/web/`：删除 chat handler 与 chat 模板；侧边栏去掉“对话”入口。
- 删除 `no_webchat` build tag 相关文件（保持构建与行为单一、可预测）。

### 2) 数据面 chat 分组约束（X-Realms-Chat）
- `internal/api/openai/handler.go`：移除 `X-Realms-Chat` 头触发的“对话分组集合”限制逻辑与相关接口注入（ChatGroupResolver）。
- `internal/store/`：删除 `chat_group_name` 设置项读取逻辑与错误类型；移除 `app_settings_defaults.chat_group_name`。
- `internal/admin/`：移除管理后台分组页“对话分组设置”相关 UI 与接口。

### 3) SearXNG 搜索（仅用于 Web chat）
- 删除 `internal/search/`（如确认仅被 Web chat 引用）。
- `internal/config/`：移除 `search.*` 配置结构与环境变量覆盖；同时移除系统设置中对 `app_settings.search_searxng_*` 的覆盖逻辑与 UI。

### 4) 数据库迁移清理
新增迁移（例如 `0038_remove_web_chat.sql`）：
- `DELETE FROM app_settings WHERE key IN (...)`：
  - `chat_group_name`
  - `feature_disable_web_chat`
  - `search_searxng_enable/search_searxng_base_url/search_searxng_timeout/search_searxng_max_results/search_searxng_user_agent`
- `DELETE FROM user_tokens WHERE name='chat'`

## 风险与规避

- 风险：误删与业务无关的 Token。
  - 规避：仅删除 `user_tokens.name='chat'`（该值由 Web chat 固定写入），不按 token_hint 或 hash 进行模糊匹配。
- 风险：有外部调用方依赖 `X-Realms-Chat: 1` 的分组约束。
  - 规避：本提案目标即删除该行为；如后续仍需该能力，应以显式配置项重新引入，并补充迁移策略。

## 验证

- `go test ./...`
- 路由测试：确认 `/chat` 与 `/api/chat/*` 返回 404。
- 回归：`POST /v1/chat/completions` 返回 404；`POST /v1/responses` 基本路径不受影响（至少编译与单测覆盖）。
