# 任务清单: 用量统计快捷区间自动刷新

目录: `helloagents/plan/202601190615_usage_quick_range_autorefresh/`

---

## 1. 修复快捷区间切换不刷新
- [√] 1.1 Web 用量页 `/usage`：快捷区间按钮点击后同步 hidden start/end 并自动提交查询
- [√] 1.2 管理后台用量页 `/admin/usage`：快捷区间按钮点击后同步 hidden start/end 并自动提交查询

## 2. 回归验证
- [√] 2.1 执行 `go test ./...`

## 3. 知识库同步
- [√] 3.1 更新 `helloagents/wiki/modules/realms.md` 与 `helloagents/CHANGELOG.md`

## 4. 迁移
- [√] 4.1 迁移方案包至 `helloagents/history/2026-01/` 并更新 `helloagents/history/index.md`
