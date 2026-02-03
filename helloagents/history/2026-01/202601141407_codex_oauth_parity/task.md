# 任务清单: Codex OAuth 对齐 CLIProxyAPI

目录: `helloagents/plan/202601141407_codex_oauth_parity/`

---

## 1. OAuth Flow 对齐（codexoauth）
- [√] 1.1 在 `internal/codexoauth/flow.go` 中补齐回调成功/失败的 HTML 响应，验证 why.md#需求-oauth-flow-parity-场景-oauth-callback-success
- [√] 1.2 在 `internal/codexoauth/flow.go` 中强化 state 一次性消费与超时处理（Pending TTL），验证 why.md#需求-oauth-flow-parity-场景-oauth-callback-error
- [√] 1.3 在 `internal/codexoauth/client.go` 中对齐 authorize URL 参数与 `S256` PKCE 行为，验证 why.md#需求-oauth-flow-parity-场景-admin-start-oauth

## 2. 刷新策略对齐（lead + retry/backoff）
- [√] 2.1 在 `internal/upstream/executor.go` 中对齐“临期刷新 lead”判定（默认 5 分钟量级），验证 why.md#需求-token-refresh-robustness-场景-near-expiry-auto-refresh
- [√] 2.2 在 `internal/codexoauth/client.go` 中为 refresh 增加有限重试与简单退避，并与 DB cooldown 协同，验证 why.md#需求-token-refresh-robustness-场景-refresh-failure-cooldown

## 3. 错误分层与后台提示
- [√] 3.1 在 `internal/codexoauth` 增加错误码与用户可读消息映射（不泄露敏感信息），验证 why.md#需求-user-friendly-errors--diagnostics-场景-callback-port-busy
- [√] 3.2 在 `internal/admin/server.go` 与 `internal/admin/templates/codex_accounts.html` 中增强错误/状态展示（包含 cooldown/disabled 等），验证 why.md#需求-user-friendly-errors--diagnostics-场景-callback-port-busy

## 4. 启动行为与回调监听健壮性
- [√] 4.1 在 `cmd/realms/main.go` 中对 `codex_oauth.enable=true` 场景下的回调监听启动失败给出明确策略（失败即退出），验证 why.md#需求-oauth-flow-parity-场景-admin-start-oauth

## 5. 安全检查
- [√] 5.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

## 6. 文档更新
- [√] 6.1 更新 `helloagents/wiki/modules/realms.md` 说明 OAuth 回调页/刷新策略与常见错误处理
- [√] 6.2 更新 `helloagents/CHANGELOG.md` 记录本次变更

## 7. 测试
- [√] 7.1 在 `internal/codexoauth` 增加单元测试：PKCE/authorize URL/state TTL
- [√] 7.2 在 `internal/upstream` 增加单元测试：刷新触发判定与 cooldown 行为
