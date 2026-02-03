# 任务清单: 对话分组可选/可关闭（不影响原功能）

目录: `helloagents/plan/202601171711_chat_group_optional/`

---

## 1. 管理后台：可关闭
- [√] 1.1 新增 `POST /admin/channel-groups/{group_id}/unset-chat`：清空 `app_settings.chat_group_name`
- [√] 1.2 分组编辑弹窗中将“对话分组”按钮改为可切换：未启用→“设为对话分组”，已启用→“关闭对话分组”

## 2. 数据面/对话页：对话分组为附加项
- [√] 2.1 `X-Realms-Chat: 1` 且未配置/已禁用对话分组时，回退为原有“按用户分组调度”逻辑（不报错）
- [√] 2.2 `/api/chat/models` 同步支持上述回退：未启用对话分组时按用户分组返回可用模型
- [√] 2.3 更新 `/chat` 页面提示文案，避免误导

## 3. 文档与验证
- [√] 3.1 更新 `helloagents/wiki/api.md`、`helloagents/CHANGELOG.md`
- [√] 3.2 执行 `go test ./...` 并确保通过
