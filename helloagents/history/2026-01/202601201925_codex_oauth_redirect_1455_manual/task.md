# 任务清单: Codex OAuth 固定 1455 回调 + 手动粘贴

目录: `helloagents/plan/202601201925_codex_oauth_redirect_1455_manual/`

---

## 1. 默认值回退到 Codex 端口
- [√] 1.1 在 `internal/config/config.go` 将默认 `codex_oauth.redirect_uri` 回退为 `http://localhost:1455/auth/callback`
- [√] 1.2 保持默认 `codex_oauth.callback_listen_addr` 为空，避免服务启动第二个监听端口
- [√] 1.3 在 `cmd/realms/main.go` 修正启动日志文案（避免误导“走主服务端口”）

## 2. 配置示例与 UI 文案
- [√] 2.1 更新 `config.example.yaml`：明确“redirect_uri=1455 + 不监听 → 需手动粘贴回调 URL”的默认行为
- [√] 2.2 更新 `internal/admin/templates/endpoints.html`：回调 URL 占位符回退到 1455

## 3. 知识库与变更记录
- [√] 3.1 更新知识库：`helloagents/wiki/api.md`、`helloagents/wiki/modules/realms.md`
- [√] 3.2 更新 `helloagents/CHANGELOG.md`：修正文案口径（默认不自动回调）

## 4. 测试
- [√] 4.1 运行 `go test ./...` 验证通过
