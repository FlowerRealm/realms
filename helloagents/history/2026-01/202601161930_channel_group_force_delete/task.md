# 轻量迭代：channel_group_force_delete（2026-01-16 19:30）

目标：允许在分组仍被引用时删除，并在删除时自动解除绑定，避免残留引用导致的不可预期行为。

## 任务清单

- [√] 允许后台删除仍被引用的分组（不再阻塞 users/channels 引用）
- [√] 删除时解绑 `user_groups`（移除所有用户对该分组的绑定）
- [√] 删除时处理 `upstream_channels.groups`（移除该分组；若移除后为空则禁用渠道并回退到 `default`）
- [√] 更新管理后台删除确认文案（明确“自动解绑/可能自动禁用”）
- [√] 更新知识库：分组删除语义与副作用说明
- [√] 更新 `helloagents/CHANGELOG.md` 记录本次变更
- [√] 运行测试：`go test ./...`
- [√] 迁移方案包到 `helloagents/history/2026-01/`
