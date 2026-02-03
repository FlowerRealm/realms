# 任务清单: 对话分组为空时关闭对话 + 精简分组页操作

目录: `helloagents/plan/202601171901_chat_disable_without_groups/`

---

## 1. 行为调整：对话分组为空即关闭对话
- [√] 1.1 数据面：当请求携带 `X-Realms-Chat: 1` 且对话分组未配置/全部禁用时，直接拒绝（不再回退到用户分组）
- [√] 1.2 Web：`GET /chat`、`GET /api/chat/models`、`POST /api/chat/token` 在对话分组未启用时返回“对话功能已关闭”

## 2. UI 精简：移除单分组快捷按钮
- [√] 2.1 管理后台 `/admin/channel-groups`：保留右上角“对话分组设置”多选入口，移除每个分组编辑弹窗内的“加入/移出对话分组”按钮

## 3. 文档与测试
- [√] 3.1 更新 `internal/api/openai/handler_test.go`：原“未配置回退用户分组”用例改为“未配置拒绝”
- [√] 3.2 更新知识库文档：`helloagents/wiki/api.md`（关闭语义），`helloagents/CHANGELOG.md`（行为变更记录）
- [√] 3.3 运行 `go test ./...` 并确保通过
