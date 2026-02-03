# 变更提案：移除 Web 对话功能（/chat）

## 背景

当前 `realms` 内置了一套 Web 对话功能（`GET /chat` + `/api/chat/*`），并配套：
- 管理后台「分组」页的“对话分组设置”（`app_settings.chat_group_name` + `X-Realms-Chat: 1` 约束调度）。
- 管理后台「系统设置」中的“联网搜索（SearXNG）”覆盖项（`app_settings.search_searxng_*`），供 Web `/chat` 通过服务端代理请求。

用户诉求是**彻底删除对话功能**，包括前端页面、后端接口与数据库中相关配置/数据的清理。

## 目标

- 移除 Web 对话页面与配套接口：
  - `GET /chat`
  - `GET /api/chat/models`
  - `POST /api/chat/token`
  - `POST /api/chat/search`
- 移除数据面对话接口：
  - `POST /v1/chat/completions`
- 移除“对话分组”能力与相关管理入口：
  - 不再支持 `X-Realms-Chat: 1` 的对话分组约束
  - 删除 `/admin/channel-groups/*chat*` 相关操作与 UI
  - 清理 `app_settings.chat_group_name`
- 移除仅为 Web 对话服务的“联网搜索（SearXNG）”覆盖能力：
  - 删除管理后台的相关设置项 UI 与逻辑
  - 清理 `app_settings.search_searxng_*`
- 数据库清理：
  - 清理 `user_tokens.name='chat'` 的对话页专用 Token（仅删除这一类 Token，不影响其他 Token）
  - 清理上述 app_settings 键

## 非目标 / 约束

- 不移除数据面 Responses：`POST /v1/responses` 仍保留（作为唯一数据面入口）。
- 不改变既有的渠道分组（`channel_groups`）与用户分组（`user_groups`）语义；仅删除“对话分组集合”这一层额外配置。

## 成功标准

- Web 控制台不再出现“对话”入口，访问 `/chat` 与 `/api/chat/*` 返回 404（路由未注册）。
- 数据面 `POST /v1/chat/completions` 返回 404（路由未注册）。
- 管理后台不再出现“对话分组设置”与“SearXNG 联网搜索（供 /chat 使用）”相关配置项。
- 代码库中不再存在 `no_webchat` 构建开关与 Web chat 相关实现文件。
- 新增迁移在升级时删除：`app_settings.chat_group_name`、`app_settings.feature_disable_web_chat`、`app_settings.search_searxng_*`、以及 `user_tokens.name='chat'`。
