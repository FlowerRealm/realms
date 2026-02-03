# 任务清单: 管理后台合并对话分组配置到分组页

目录: `helloagents/plan/202601171640_chat_group_merge/`

---

## 1. 管理后台 UI 合并
- [√] 1.1 移除独立页面 `/admin/chat-routes` 与侧边栏入口
- [√] 1.2 在 `/admin/channel-groups` 列表中为每个分组增加“设为对话分组”按钮，并展示当前对话分组
- [√] 1.3 增加后端接口 `POST /admin/channel-groups/{group_id}/set-chat` 写入对话分组设置

## 2. 对话路由逻辑调整
- [√] 2.1 使用 `app_settings.chat_group_name` 作为对话分组唯一配置来源（无需 `chat_group_routes` 表）
- [√] 2.2 `X-Realms-Chat: 1` 时按对话分组限制可用渠道（组内可 failover），并在模型不在对话分组时明确报错
- [√] 2.3 `/api/chat/models` 改为按对话分组过滤模型

## 3. 清理与验证
- [√] 3.1 删除不再使用的 `chat_group_routes` 迁移/Store/Admin 代码与测试
- [√] 3.2 更新 `helloagents/wiki/api.md`、`helloagents/wiki/data.md`、`helloagents/CHANGELOG.md`
- [√] 3.3 执行 `go test ./...` 并确保通过
