# 任务清单: 用户对话功能（按分组固定渠道）

目录: `helloagents/plan/202601171504_chat_ui/`

---

## 1. 数据与存储（chat_group_routes）
- [√] 1.1 新增迁移 `internal/store/migrations/0030_chat_group_routes.sql` 创建表 `chat_group_routes`，验证 why.md#需求-管理员配置对话分组路由-场景-root-为某分组绑定渠道
- [√] 1.2 在 `internal/store/` 增加对话路由的读写方法（list/get/upsert/delete + “按用户组选择渠道”），验证 why.md#需求-对话请求强制走分组绑定渠道-场景-对话请求命中固定渠道

## 2. 管理后台：对话分组路由配置
- [√] 2.1 在 `internal/admin/` 新增对话路由管理 handler（`GET /admin/chat-routes` + `POST /admin/chat-routes` + delete），验证 why.md#需求-管理员配置对话分组路由-场景-root-为某分组绑定渠道
- [√] 2.2 在 `internal/admin/templates/` 新增页面模板并接入侧边栏导航，验证 why.md#需求-管理员配置对话分组路由-场景-root-为某分组绑定渠道

## 3. 用户侧：对话页面与 token 自动化
- [√] 3.1 在 `internal/web/` 新增 `GET /chat` 页面（SSR）与模板，包含：模型选择、对话框、清空本地记录按钮，验证 why.md#需求-用户可在-web-前端对话-场景-登录后开始一次流式对话
- [√] 3.2 新增 `POST /api/chat/token`（会话鉴权 + CSRF）：首次发放/可选轮换 `chat` token，验证 why.md#需求-自动创建-chat-token-并在浏览器使用-场景-首次进入对话页自动可用
- [√] 3.3 新增 `GET /api/chat/models`：按“当前用户对话渠道”过滤可用模型，验证 why.md#需求-对话请求强制走分组绑定渠道-场景-对话请求命中固定渠道
- [√] 3.4 前端 JS：使用 `localStorage` 保存会话与 `chat` token；对话请求调用 `POST /v1/responses` 且携带 `X-Realms-Chat: 1`；401 时触发 token 轮换，验证 why.md#需求-自动创建-chat-token-并在浏览器使用-场景-token-丢失或失效可自助恢复

## 4. 数据面：对话请求固定渠道约束
- [√] 4.1 在 `internal/api/openai/handler.go` 中支持对话标识（`X-Realms-Chat: 1`）并强制 `RequireChannelID`，验证 why.md#需求-对话请求强制走分组绑定渠道-场景-对话请求命中固定渠道
- [√] 4.2 补充单元测试覆盖“对话标识命中固定渠道/模型未绑定报错/默认回退”，验证 why.md 相关场景

## 5. 安全检查
- [√] 5.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、CSRF、防越权路由、避免 token 泄漏到日志/URL）

## 6. 文档更新
- [√] 6.1 更新 `helloagents/wiki/api.md` 补充对话相关接口与行为
- [√] 6.2 更新 `helloagents/wiki/data.md` 补充 `chat_group_routes` 表

## 7. 测试
- [√] 7.1 执行 `go test ./...` 并确保通过（允许为新功能补充测试数据/测试桩）
