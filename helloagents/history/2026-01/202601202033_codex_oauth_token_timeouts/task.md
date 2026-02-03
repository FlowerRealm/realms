# 任务清单: Codex OAuth token 换取超时与重试

目录: `helloagents/plan/202601202033_codex_oauth_token_timeouts/`

---

## 1. Codex OAuth HTTP 客户端
- [√] 1.1 为 Codex OAuth token exchange 增加可配置的 HTTP/TLS 握手超时，并规避 DefaultTransport 类型断言的潜在 panic
- [√] 1.2 对明确的拨号失败/TLS 握手超时做一次安全重试，降低偶发网络抖动导致的授权失败率

## 2. 文档更新
- [√] 2.1 更新 `config.example.yaml` 与知识库中 Codex OAuth 配置说明（含代理/网络排查与超时参数）

## 3. 测试
- [√] 3.1 运行 `go test ./...`
