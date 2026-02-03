# How: 实现方案（username 必填 + 数据迁移）

## 1. 应用层约束

- 统一使用 `store.NormalizeUsername` 做输入规范化与校验：
  - 必填：空值直接报错。
  - 规范化：trim + toLower。
  - 约束：长度 ≤ 32，字符集 `[a-z0-9._-]` 且以字母/数字开头。
- Web 注册、账号设置、管理后台创建/编辑用户：
  - 表单字段增加 `required`。
  - 服务端强制校验并保持“账号名已被占用”的友好提示。

## 2. 数据库迁移

- 新增迁移 `0029_users_username_required.sql`：
  - 将 `username IS NULL OR username=''` 的行回填为 `u{id}_{md5}` 形式（满足校验规则，尽量避免与存量冲突）。
  - 将 `users.username` 改为 `NOT NULL`。
  - 兜底确保唯一索引 `uk_users_username` 存在。

## 3. 兼容性与风险

- 风险：极小概率与已有 username 冲突（需用户手动设置为迁移生成的同名字符串）。
  - 处理：迁移依赖唯一索引；发生冲突会在迁移阶段失败并阻止启动，便于及时发现与人工处理。

