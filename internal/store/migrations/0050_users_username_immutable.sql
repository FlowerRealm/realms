-- 0050_users_username_immutable.sql: users.username 收敛为不可更改 + 唯一 + 区分大小写 + 仅字母/数字。

-- 1) 存量修复：将不合规 username（空串/含非字母数字字符）回填为稳定且唯一的 `uid{id}{md5}`。
UPDATE users
SET username = CONCAT('uid', id, SUBSTRING(MD5(CONCAT(email, id)), 1, 8))
WHERE username IS NULL OR username = '' OR username REGEXP '[^A-Za-z0-9]';

-- 2) username 改为 case-sensitive（utf8mb4_bin）；唯一索引将随之变为大小写敏感。
ALTER TABLE `users`
  MODIFY COLUMN `username` VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL;

-- 3) 账号名格式约束：仅允许字母/数字（区分大小写），不允许空格或特殊字符。
ALTER TABLE `users`
  ADD CONSTRAINT `chk_users_username_alnum` CHECK (`username` REGEXP '^[A-Za-z0-9]+$');

-- 4) 数据库层不可更改：任何 UPDATE 都强制保持旧的 username（避免触发器体含分号导致迁移执行器切分失败）。
DROP TRIGGER IF EXISTS `trg_users_username_immutable`;
CREATE TRIGGER `trg_users_username_immutable`
BEFORE UPDATE ON `users`
FOR EACH ROW SET NEW.`username` = OLD.`username`;

