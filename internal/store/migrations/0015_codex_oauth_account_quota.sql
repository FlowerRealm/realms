-- 0015_codex_oauth_account_quota.sql: Codex OAuth 账号订阅额度/速率限制（后台缓存/展示）。

ALTER TABLE codex_oauth_accounts
  ADD COLUMN quota_credits_has_credits TINYINT NULL,
  ADD COLUMN quota_credits_unlimited TINYINT NULL,
  ADD COLUMN quota_credits_balance VARCHAR(64) NULL,
  ADD COLUMN quota_primary_used_percent INT NULL,
  ADD COLUMN quota_primary_reset_at DATETIME NULL,
  ADD COLUMN quota_secondary_used_percent INT NULL,
  ADD COLUMN quota_secondary_reset_at DATETIME NULL,
  ADD COLUMN quota_updated_at DATETIME NULL,
  ADD COLUMN quota_error VARCHAR(255) NULL;

