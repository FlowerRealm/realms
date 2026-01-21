-- 0013_codex_oauth_account_balance.sql: Codex OAuth 账号额度（后台缓存/展示）。

ALTER TABLE codex_oauth_accounts
  ADD COLUMN balance_total_granted_usd DECIMAL(20,6) NULL,
  ADD COLUMN balance_total_used_usd DECIMAL(20,6) NULL,
  ADD COLUMN balance_total_available_usd DECIMAL(20,6) NULL,
  ADD COLUMN balance_updated_at DATETIME NULL,
  ADD COLUMN balance_error VARCHAR(255) NULL;
