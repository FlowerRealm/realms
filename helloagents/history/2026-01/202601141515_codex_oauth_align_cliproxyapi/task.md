# 任务清单: Codex OAuth 对齐 CLIProxyAPI

目录: `helloagents/history/2026-01/202601141515_codex_oauth_align_cliproxyapi/`

---

## 1. claims 解析对齐（codexoauth）
- [√] 1.1 在 `internal/codexoauth/jwt.go` 中实现结构化 claims 解析（支持 `https://api.openai.com/auth`），验证 why.md#需求-id-token-claims-对齐-场景-oauth-callback-successclaims-解析
- [√] 1.2 在 `internal/codexoauth/flow.go` 中使用新 claims 解析提取 `chatgpt_account_id/email` 并入库，验证 why.md#需求-id-token-claims-对齐-场景-oauth-callback-successclaims-解析
- [√] 1.3 在 `internal/admin/server.go` 中对齐 `plan_type/subscription_active_*` 的展示来源（复用 codexoauth 解析），验证 why.md#需求-id-token-claims-对齐-场景-oauth-callback-successclaims-解析

## 2. 管理后台提示与兜底（admin）
- [√] 2.1 在 `internal/admin/templates/codex_accounts.html` 中增加 `localhost:1455` 回调说明与 SSH 端口转发提示，验证 why.md#需求-管理后台可用性与提示-场景-远程访问管理后台localhost-回调限制
- [√] 2.2（可选）在 `internal/admin/server.go` 增加“粘贴回调 URL 完成授权”的 handler（解析 code/state/error 并复用同一授权逻辑），验证 why.md#需求-管理后台可用性与提示-场景-远程访问管理后台localhost-回调限制

## 3. 安全检查
- [√] 3.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

## 4. 文档更新
- [√] 4.1 更新 `helloagents/wiki/modules/codex.md`：补充 Realms 的 OAuth 使用说明（claims 解析/远程回调提示/兜底流程）
- [√] 4.2 更新 `helloagents/CHANGELOG.md` 记录本次变更

## 5. 测试
- [√] 5.1 在 `internal/codexoauth` 增加单元测试：claims 解析覆盖嵌套结构与兜底分支
- [√] 5.2（可选）为回调 URL 解析与手动完成 handler 增加测试
