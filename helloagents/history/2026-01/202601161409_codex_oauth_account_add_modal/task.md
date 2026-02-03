# 任务清单: Codex OAuth 账号添加入口收敛到「账号列表」右上角

目录: `helloagents/plan/202601161409_codex_oauth_account_add_modal/`

---

## 1. 管理后台 UI 调整
- [√] 1.1 将 Codex OAuth 账号添加入口改为「账号列表」右上角 `+` 按钮（弹窗内含快捷授权/手工录入）
- [√] 1.2 调整 `#accounts` 锚点指向账号列表，保持“管理授权”跳转可用

## 2. 文档同步
- [√] 2.1 更新 `helloagents/wiki/modules/realms.md`（Codex OAuth 入口与添加方式）
- [√] 2.2 更新 `helloagents/CHANGELOG.md` 记录变更
- [√] 2.3 更新 `helloagents/history/index.md` 增加变更索引

## 3. 测试
- [√] 3.1 运行 `go test ./...`
