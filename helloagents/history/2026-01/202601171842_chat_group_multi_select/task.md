# 任务清单: 对话分组支持多选

目录: `helloagents/plan/202601171842_chat_group_multi_select/`

---

## 1. store（对话分组解析）
- [√] 1.1 在 `internal/store/chat_group.go` 中实现 `ResolveChatGroupNames(ctx)`：支持从 `app_settings.chat_group_name` 解析 CSV 并过滤启用分组
- [√] 1.2 更新相关调用方使用“分组集合”语义（而非单个分组名）

## 2. openai handler（对话路由约束）
- [√] 2.1 在 `internal/api/openai/handler.go` 中将对话分组约束从单组改为多组集合
- [√] 2.2 在 `internal/api/openai/handler_test.go` 中更新 fake resolver 与既有用例，并新增“多分组并集可用”的测试

## 3. web（对话模型列表与文案）
- [√] 3.1 在 `internal/web/chat.go` 中，已启用对话分组时按分组集合过滤模型（并集）
- [√] 3.2 在 `internal/web/templates/chat.html` 中更新对话分组相关文案为“分组集合”

## 4. admin（后台多选）
- [√] 4.1 在 `internal/admin/channel_groups.go` 新增保存对话分组集合的 handler（checkbox 多选）
- [√] 4.2 在 `internal/admin/templates/channel_groups.html` 增加“对话分组设置”弹窗与多选 UI
- [√] 4.3 调整 `set-chat/unset-chat/delete` 对 CSV 集合的兼容处理

## 5. 安全检查
- [√] 5.1 检查分组名输入校验、SQL 拼接与错误处理，确保不存在注入与越权风险

## 6. 文档更新
- [√] 6.1 更新 `helloagents/wiki/api.md`：对话分组从单组扩展为多组集合（CSV）
- [√] 6.2 更新 `helloagents/wiki/data.md`：`app_settings.chat_group_name` 含义更新为 CSV 列表
- [√] 6.3 更新 `helloagents/CHANGELOG.md`：记录本次变更

## 7. 测试
- [√] 7.1 运行 `go test ./...` 并确保通过
