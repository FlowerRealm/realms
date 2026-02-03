# 变更提案: username-immutable

## 元信息
```yaml
类型: 数据库变更
方案类型: implementation
优先级: P1
状态: 草稿
创建: 2026-02-03
```

---

## 1. 需求

### 背景
当前系统 `users.username` 已存在并用于登录（邮箱或账号名）。但现状存在：
- 账号名可被用户/管理员修改（API 与 UI 均支持），与“账号名作为稳定标识”目标冲突。
- 账号名当前规范化为全小写（`NormalizeUsername`），无法满足“区分大小写”的唯一性要求。
- 数据库层缺少“仅字母/数字、不允许空格与特殊字符”的硬约束（部分环境可能存在历史遗留账号名）。

### 目标
- 将 `username` 作为与 `email` 同级的稳定字段：
  - **不可更改**（应用层禁用 + 数据库层约束）
  - **唯一**（数据库唯一索引）
  - **区分大小写**（数据库比较/索引为 case-sensitive；应用层不再强制 lower）
  - **仅允许字母/数字**（禁止空格与特殊字符）
- 提供数据库迁移：
  - MySQL：新增迁移脚本完成约束收敛与存量数据修复
  - SQLite：初始化 schema 与运行期 ensure 逻辑同步收敛

### 约束条件
```yaml
兼容性约束: 不做过多原有兼容逻辑（允许 API 行为变化）
数据库: SQLite（默认）+ MySQL 8.x（可选）
```

### 验收标准
- [ ] 注册：创建用户时 `username` 必填，且仅允许字母/数字（区分大小写）。
- [ ] 登录：允许用 `email` 或 `username` 登录；`username` 登录为大小写敏感。
- [ ] 账号名修改：用户侧与管理员侧均无法修改 `username`（API 返回明确错误；数据库层保证不会被更改）。
- [ ] MySQL：迁移后 `username` 唯一、case-sensitive，并且新增写入被数据库约束拦截。
- [ ] SQLite：新库初始化具备同等约束；旧库启动后自动补齐约束。

---

## 2. 方案

### 数据库方案

#### MySQL 8.x（迁移）
- 目标形态：
  - `users.username`：`VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL`
  - `uk_users_username`：唯一索引（随列 collation 变更为 case-sensitive）
  - `CHECK`：`username` 仅允许 `[A-Za-z0-9]+`
  - `TRIGGER`（before update）：将 `NEW.username` 强制回滚为 `OLD.username`，实现数据库层不可更改
- 存量修复：
  - 将不满足新规则的账号名（空串或含非字母数字字符）回填为稳定且唯一的 `u{id}{md5}`（仅字母/数字）。

#### SQLite（初始化 + ensure）
- `schema_sqlite.sql`：补齐 username 规则（使用 trigger 校验插入 + trigger 拦截更新）。
- `EnsureSQLiteSchema`：对已有 SQLite 库补齐触发器；并在首次补齐前修复存量不合规账号名（回填为 `uid{id}`）。

### 应用层方案
- 账号名规范化：
  - `NormalizeUsername`：不再 `ToLower`；只做 trim + 校验（字母/数字，最大 64）。
- API：
  - 禁用用户侧 `/api/account/username` 的修改能力（返回“账号名不可修改”）。
  - 管理员侧 `PUT /api/admin/users/{user_id}` 不再接受 `username` 更新（若提交则返回“账号名不可修改”）。
  - 登录接口：不再将账号名统一转小写；邮箱仍按小写处理。
- Web（SPA）：
  - 账号设置页：账号名改为只读展示，移除“修改账号名”交互。
  - 管理后台用户编辑：账号名只读展示，不再提供编辑入口。

### 影响范围
```yaml
涉及模块:
  - internal/store/: username 规则、SQLite schema ensure、MySQL migrations
  - router/: 登录与账号/管理员接口调整
  - web/: 账号设置页与管理员用户页 UI/API 调整
  - tests/: 更新测试数据（移除 '-' '_' '.' 等不再允许的账号名）
预计变更文件: 10-20
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 存量用户名包含特殊字符，迁移时触发约束失败 | 中 | 迁移中先修复存量，再加约束 |
| 前端/脚本使用旧的“可修改 username”接口 | 中 | API 返回明确错误；同步更新 SPA 与知识库文档 |
| MySQL 触发器语义与迁移执行器（分号切分）冲突 | 中 | MySQL 触发器使用单语句形式（不含 BEGIN/END） |

---

## 3. 核心场景

### 场景 1：用户注册后账号名不可修改
- 注册：提交 `email/username/password` 创建用户成功。
- 用户尝试调用 `/api/account/username`：返回失败（账号名不可修改）。
- 数据库层：任何更新语句都无法改变 `users.username`。

### 场景 2：区分大小写的账号名唯一性
- 可同时存在 `Alice` 与 `alice` 两个账号（仅当两者均未被占用）。
- 使用账号名登录必须严格匹配大小写（否则登录失败）。

