# 任务清单: Codex OAuth 回调完成后刷新原页面（轻量迭代）

目录: `helloagents/plan/202601142120_oauth_callback_refresh_opener/`

---

## 1. 回调页行为
- [√] 1.1 `internal/codexoauth/html.go`：回调页优先通过 `postMessage` 通知 opener 刷新/跳转到管理页（展示新账号），并尝试自动关闭回调窗口

## 2. 管理后台接收刷新信号
- [√] 2.1 `internal/admin/templates/base.html`：监听 OAuth 回调消息，校验 `redirectURL` 同源后跳转/刷新

## 3. 文档与验证
- [√] 3.1 更新 `helloagents/CHANGELOG.md`
- [√] 3.2 运行 `go test ./...`
