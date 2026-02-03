# 任务清单: username-immutable

目录: `helloagents/plan/202602032206_username_immutable/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 14
已完成: 14
完成率: 100%
```

---

## 任务列表

### 1. 数据库（MySQL 8.x）
- [√] 1.1 新增迁移：`users.username` 设为 case-sensitive collation（utf8mb4_bin）
- [√] 1.2 存量修复：回填不合规 username（空串/含特殊字符）
- [√] 1.3 增加 CHECK：限制 username 仅 `[A-Za-z0-9]+`
- [√] 1.4 增加触发器：阻止 username 被更新（强制回滚为旧值）

### 2. 数据库（SQLite）
- [√] 2.1 更新 `schema_sqlite.sql`：补齐 username 校验（CHECK + 索引 collation）
- [√] 2.2 新增 ensure：旧库启动时补齐触发器与存量修复（insert 校验 + update 拦截）

### 3. 后端（Go）
- [√] 3.1 `NormalizeUsername`：移除 lower，改为大小写敏感 + 仅字母/数字
- [√] 3.2 登录：账号名不再统一转小写；邮箱仍按小写处理
- [√] 3.3 禁用账号设置：`/api/account/username` 返回“账号名不可修改”
- [√] 3.4 管理后台：`PUT /api/admin/users/{user_id}` 不再允许更新 username

### 4. 前端（SPA）
- [√] 4.1 账号设置页：账号名改为只读展示，移除修改入口
- [√] 4.2 管理后台用户页：编辑弹窗不再提供账号名编辑

### 5. 测试与文档
- [√] 5.1 更新测试数据：替换含 `- _ .` 的账号名
- [√] 5.2 增加/更新单测：覆盖 `NormalizeUsername`（大小写/非法字符/长度）
- [√] 5.3 同步知识库文档：账号体系与 admin_users 模块描述更新
- [√] 5.4 更新 `helloagents/CHANGELOG.md`：记录本次变更
