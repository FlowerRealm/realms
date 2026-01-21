-- 0033_channel_group_members.sql: 渠道组树形路由（channel_group_members）与组内尝试次数（max_attempts）。

-- 1) channel_groups 增加 max_attempts（组内成员 failover 次数上限；默认 5）
ALTER TABLE `channel_groups`
  ADD COLUMN `max_attempts` INT NOT NULL DEFAULT 5 AFTER `status`;

-- 2) 新增成员关系表：组 →（子组/叶子渠道），并支持排序字段（promotion/priority）
CREATE TABLE IF NOT EXISTS `channel_group_members` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `parent_group_id` BIGINT NOT NULL,
  `member_group_id` BIGINT NULL,
  `member_channel_id` BIGINT NULL,
  `priority` INT NOT NULL DEFAULT 0,
  `promotion` TINYINT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_parent_member_group` (`parent_group_id`, `member_group_id`),
  UNIQUE KEY `uk_parent_member_channel` (`parent_group_id`, `member_channel_id`),
  UNIQUE KEY `uk_group_single_parent` (`member_group_id`),
  KEY `idx_parent_order` (`parent_group_id`, `promotion`, `priority`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 3) 回填：default 根组挂载所有现有组（不含 default 自身）
INSERT IGNORE INTO `channel_group_members` (`parent_group_id`, `member_group_id`, `priority`, `promotion`, `created_at`, `updated_at`)
SELECT d.id, g.id, 0, 0, NOW(), NOW()
FROM `channel_groups` d
JOIN `channel_groups` g ON g.name <> 'default'
WHERE d.name = 'default';

-- 4) 回填：现有 upstream_channels.groups（CSV）→ 组成员（组 -> 渠道）
-- 兼容：空/脏 groups 按 default 处理；并容忍历史上写入的空格（统一去空格）。
INSERT IGNORE INTO `channel_group_members` (`parent_group_id`, `member_channel_id`, `priority`, `promotion`, `created_at`, `updated_at`)
SELECT cg.id, uc.id, uc.priority, uc.promotion, NOW(), NOW()
FROM `channel_groups` cg
JOIN `upstream_channels` uc
  ON FIND_IN_SET(
       cg.name,
       REPLACE(
         IF(TRIM(uc.`groups`)='', 'default', TRIM(uc.`groups`)),
         ' ',
         ''
       )
     ) > 0;

