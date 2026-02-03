# 轻量迭代任务清单：codex_oauth_failover_402

- [√] 将 `402 Payment Required` 视为可重试状态，触发 failover/冷却（对齐 CLIProxyAPI 的“payment_required 可切换账号”行为）
- [√] 补充单测：`/v1/responses` 在上游返回 402 时会切换到下一个 credential
- [√] 同步知识库：记录 failover 的状态码口径（含 402）
- [√] 更新 `helloagents/CHANGELOG.md`
- [√] 迁移方案包至 `helloagents/history/2026-01/` 并更新 `helloagents/history/index.md`
