# 技术设计: 用户账号管理（账号名/邮箱/密码）

## 技术方案

### 核心技术
- Go（`net/http` + SSR 模板）
- MySQL 迁移（`internal/store/migrations/*.sql`）
- bcrypt（密码哈希）
- 现有 CSRF 中间件（`internal/middleware/csrf.go`）
- 现有邮箱验证码发送接口（`POST /api/email/verification/send`）

### 实现要点
- 数据层：
  - `users` 表新增 `username VARCHAR(64) NULL`，并建立唯一索引（允许多个 NULL）。
  - store 层补齐 `username` 字段读写、按 `username` 查询、更新 email/username/password、按 user_id 清理 sessions。
- 登录：
  - 登录表单字段从“邮箱”扩展为“邮箱或账号名”，后端优先按 email 查询，未命中再按 username 查询。
  - 保持兼容：仍接受旧表单字段名（如 `email`）。
- Web 控制台（普通用户）：
  - 新增账号设置页：修改账号名、修改邮箱（验证码）、修改密码（旧密码）。
  - 所有更新成功后强制登出（删除该用户全部 sessions + 清 Cookie）。
- 管理后台（root）：
  - `/admin/users` 页面增加账号名展示与编辑能力。
  - 新增 profile 更新与密码重置入口；修改成功后清理目标用户 sessions（强制登出）。
- 校验规则：
  - `username` 可为空；非空时要求唯一；限制为 1~32 位，字符集为 `[a-z0-9._-]`（统一 lower-case）。
  - email 更新需要验证码功能开启；且新邮箱未被占用；验证码通过后才写入。
  - 普通用户改密码必须校验旧密码；新密码复用现有 bcrypt 最小长度约束（>=8）。

## 数据模型

```sql
ALTER TABLE users ADD COLUMN username VARCHAR(64) NULL;
ALTER TABLE users ADD UNIQUE KEY uk_users_username (username);
```

## 安全与性能
- **安全:**
  - CSRF：所有有副作用的 Web/Admin 操作使用现有 CSRF middleware。
  - 权限：普通用户仅允许修改自己；管理员（root）可修改任意用户。
  - 邮箱变更：强制验证码；未启用邮箱验证码则拒绝变更。
  - 会话：账号信息变更后统一清理该用户所有会话（强制登出）。
- **性能:**
  - username 使用唯一索引加速查询；登录只做一次 email 查询 + 必要时一次 username 查询。

## 测试与部署
- **测试:** `go test ./...`（确保编译、模板 embed、迁移文件都可用）
- **部署:** 按现有启动流程自动执行迁移；上线前建议备份数据库并在灰度环境验证登录/修改流程。
