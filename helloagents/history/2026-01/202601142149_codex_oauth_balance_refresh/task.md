# 任务清单: Codex OAuth 账号额度自动刷新（后台）

目录: `helloagents/plan/202601142149_codex_oauth_balance_refresh/`

---

## 1. 数据层（store + migrations）
- [√] 1.1 增加迁移：为 `codex_oauth_accounts` 添加额度字段（usd_micros + updated_at + error），验证 why.md#需求-定时刷新所有账号额度-场景-自动刷新
- [√] 1.2 扩展 `store.CodexOAuthAccount` 与相关查询/更新方法，验证 why.md#需求-后台展示-codex-oauth-账号剩余额度-场景-查看账号余额

## 2. Codex OAuth 上游额度拉取（codexoauth）
- [√] 2.1 实现 credit_grants 拉取与解析（total_* → usd_micros），验证 why.md#需求-定时刷新所有账号额度-场景-自动刷新

## 3. 后台定时刷新（server）
- [√] 3.1 在 `internal/server/app.go` 增加 10 分钟刷新循环，遍历所有账号并更新额度，验证 why.md#需求-定时刷新所有账号额度-场景-自动刷新

## 4. 管理后台展示（admin）
- [√] 4.1 在 `/admin/channels/{id}/endpoints#accounts` 账号列表新增“额度/更新时间/错误”展示，验证 why.md#需求-后台展示-codex-oauth-账号剩余额度-场景-查看账号余额

## 5. 安全检查
- [√] 5.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

## 6. 文档更新
- [√] 6.1 更新 `helloagents/wiki/modules/realms.md` 记录“账号额度自动刷新”能力与口径说明
- [√] 6.2 更新 `helloagents/CHANGELOG.md` 记录本次新增

## 7. 测试
- [√] 7.1 在 `internal/codexoauth` 增加单元测试：credit_grants 解析与错误处理
- [√] 7.2 执行 `go test ./...`
