# 任务清单: 渠道分组保存返回 400「参数错误」

目录: `helloagents/plan/202601180737_channels_groups_param_error/`

---

## 1. 问题修复
- [√] 1.1 修复管理后台：分组表单即使误提交到 `POST /admin/channels` 也能按“更新分组”处理（兼容缺少 path 参数的情况）
- [√] 1.2 修复 Channels 页分组弹窗：补齐 `channel_id` 隐藏字段，避免触发 CreateChannel 的参数校验

## 2. 构建与回归
- [√] 2.1 修复 `internal/server/app.go` 调用 `web.NewServer` 参数缺失导致的构建失败
- [√] 2.2 运行 `go test ./...` 通过

## 3. 变更记录
- [√] 3.1 更新 `helloagents/CHANGELOG.md`（Unreleased）
- [√] 3.2 迁移方案包至 `helloagents/history/2026-01/202601180737_channels_groups_param_error/` 并更新 `helloagents/history/index.md`

