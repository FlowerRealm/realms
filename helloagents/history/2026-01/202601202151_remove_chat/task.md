# 任务清单：移除对话功能（Web /chat + 数据面 /v1/chat/completions）

目录：`helloagents/plan/202601202151_remove_chat/`

## 1. 删除 Web chat 页面与路由
- [√] 1.1 移除 `/chat` 与 `/api/chat/*` 路由注册与 handler（包括 build tag 文件）
- [√] 1.2 删除 Web 模板中的“对话”入口与 chat 模板文件
- [√] 1.3 新增/调整路由测试：`/chat` 与 `/api/chat/*` 始终 404

## 2. 移除对话分组（X-Realms-Chat + chat_group_name）
- [√] 2.1 删除 OpenAI handler 中 `X-Realms-Chat` 相关分组约束与接口注入
- [√] 2.2 删除 store/admin 中 `chat_group_name` 的读取、写入与管理入口
- [√] 2.3 更新相关单测，移除对该行为的断言

## 3. 移除 SearXNG 搜索覆盖（仅供 Web chat）
- [√] 3.1 删除 `internal/search/` 与相关引用
- [√] 3.2 移除 config 与 env overrides 中的 `search.searxng.*`
- [√] 3.3 移除管理后台系统设置页中 `search_searxng_*` 的展示/保存/重置逻辑

## 4. 数据库清理迁移
- [√] 4.1 新增迁移清理 `app_settings` 中 chat/search 相关键
- [√] 4.2 新增迁移清理 `user_tokens.name='chat'`

## 5. 文档与知识库同步
- [√] 5.1 更新 `README.md` 与 `config.example.yaml`，移除 Web chat 相关说明与配置
- [√] 5.2 更新 `helloagents/wiki/*`（api/data/modules）与 `helloagents/CHANGELOG.md`

## 6. 质量与交付
- [√] 6.1 gofmt（如有需要）并运行 `go test ./...`
- [√] 6.2 迁移方案包至 `helloagents/history/2026-01/` 并更新 `helloagents/history/index.md`

## 7. 下线数据面 Chat Completions（/v1/chat/completions）
- [√] 7.1 移除 `/v1/chat/completions` 路由注册与 handler
- [√] 7.2 移除 stream/超时等逻辑中对该路径的特殊处理
- [√] 7.3 更新文档与测试，确保 `/v1/chat/completions` 始终 404
