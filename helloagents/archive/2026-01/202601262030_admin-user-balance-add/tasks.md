# 任务清单: admin-user-balance-add

> **@status:** completed | 2026-01-26 20:45

目录: `helloagents/archive/2026-01/202601262030_admin-user-balance-add/`

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
总任务: 8
已完成: 8
完成率: 100%
```

---

## 任务列表

### 1. 后端（Store + Admin）

- [√] 1.1 在 `internal/store/user_balances.go` 中实现余额批量读取与加值
  - 目标: `GetUserBalancesUSD`、`AddUserBalanceUSD`
  - 验证: `go test ./...` 通过

- [√] 1.2 在 `internal/admin` 中新增“加余额”处理器
  - 目标: `POST /admin/users/{user_id}/balance`（支持 AJAX 与重定向）
  - 依赖: 1.1
  - 验证: `go test ./...` 通过

- [√] 1.3 在 `internal/server/app.go` 注册新路由
  - 依赖: 1.2
  - 验证: `go test ./...` 通过

### 2. 前端（管理后台页面）

- [√] 2.1 在 `internal/admin/templates/users.html` 增加余额展示与“加余额”入口
  - 验证: `go test ./...` 通过（模板编译/引用字段无误）

### 3. 测试

- [√] 3.1 在 `tests/store/user_balances_test.go` 增加单元测试（SQLite）
  - 覆盖: 新用户无余额记录时入账、批量读取缺失视为 0、精度截断
  - 验证: `go test ./tests/...` 与 `go test ./...` 通过

### 4. 知识库同步

- [√] 4.1 更新 `helloagents/modules/_index.md` 并新增 `helloagents/modules/admin_users.md`
  - 内容: 记录“用户余额手动加值”入口、路由、关键约束

- [√] 4.2 更新 `helloagents/CHANGELOG.md`
  - 记录: 新增管理后台手动加余额能力；补充 SQLite 余额精度修复说明

- [√] 4.3 归档方案包 `helloagents/plan/202601262030_admin-user-balance-add/`
  - 工具: `python -X utf8 "/home/flowerrealm/.codex/skills/helloagents/scripts/migrate_package.py" "202601262030_admin-user-balance-add" --status completed`

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 1.1 | completed | SQLite 下对 `user_balances` 的加减统一使用 `ROUND(..., 6)`，避免浮点误差导致 1e-6 级别入账/返还显示异常 |
| 1.2 | completed | 新增 `/admin/users/{user_id}/balance`；支持 AJAX 与重定向；写入 `audit_events`（`action=admin.user_balance.add`） |
| 2.1 | completed | `/admin/users` 新增余额列与“加余额”弹窗入口 |
