-- 0002_email_verifications_index_email_nullable_user_id.sql: 支持注册前验证码（user_id 可为空）并加速按 email 查询。

ALTER TABLE `email_verifications`
  MODIFY COLUMN `user_id` BIGINT NULL;

ALTER TABLE `email_verifications`
  ADD KEY `idx_email_verifications_email` (`email`);

