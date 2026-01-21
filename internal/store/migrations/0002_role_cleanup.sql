-- 0002_role_cleanup.sql: 移除重复的管理员角色（group_admin/admin），统一降级为 user。

UPDATE `users` SET `role`='user' WHERE `role` IN ('group_admin', 'admin');
