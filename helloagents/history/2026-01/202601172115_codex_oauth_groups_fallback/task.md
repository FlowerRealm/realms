# 任务清单: Codex OAuth 渠道分组保存误触发 base_url 限制

目录: `helloagents/plan/202601172115_codex_oauth_groups_fallback/`

---

## 1. 问题修复
- [√] 1.1 修复管理后台：当“保存分组”请求误提交到 `/admin/channels/{id}/endpoints` 时，自动回退为分组更新，避免错误提示“codex_oauth 不允许修改 base_url”

## 2. 测试与回归
- [√] 2.1 增加单元测试覆盖分组表单识别逻辑
- [√] 2.2 运行 `go test ./...` 通过

## 3. 变更记录
- [√] 3.1 更新 `helloagents/CHANGELOG.md`（Unreleased）
- [√] 3.2 迁移方案包至 `helloagents/history/2026-01/202601172115_codex_oauth_groups_fallback/` 并更新 `helloagents/history/index.md`

