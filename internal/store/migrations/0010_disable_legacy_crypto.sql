UPDATE openai_compatible_credentials
SET status=0, updated_at=NOW();

UPDATE codex_oauth_accounts
SET status=0, updated_at=NOW();

UPDATE user_tokens
SET status=0, revoked_at=COALESCE(revoked_at, NOW())
WHERE status=1;

DELETE FROM user_sessions;
