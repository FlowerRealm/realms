# 任务清单: remove_upstream_limits

目录: `helloagents/plan/202601270704_remove-upstream-limits/`

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

### 1. 数据库与迁移

- [√] 1.1 新增 MySQL 迁移删除限额列：`internal/store/migrations/0047_remove_upstream_limits.sql`
  - 验证: `go test ./...`；迁移为信息架构检测 + 可重入 DDL

- [√] 1.2 将历史限额迁移改为占位，并调整后续迁移列位置
  - 文件: `internal/store/migrations/0034_upstream_channels_limits.sql`、`internal/store/migrations/0036_account_limits_sessions_rpm_tpm.sql`、`internal/store/migrations/0041_upstream_channels_request_policy.sql`

- [√] 1.3 Anthropic credentials 表迁移移除限额列
  - 文件: `internal/store/migrations/0039_anthropic_credentials.sql`

### 2. SQLite 初始化 schema

- [√] 2.1 移除 SQLite schema 中的限额列与 seed 语句字段
  - 文件: `internal/store/schema_sqlite.sql`

### 3. Store / Scheduler / Admin 清理

- [√] 3.1 Store 移除限额字段与 SQL 读写
  - 文件: `internal/store/models.go`、`internal/store/upstreams.go`、`internal/store/admin_export_import.go`

- [√] 3.2 Scheduler 移除按限额过滤候选逻辑
  - 文件: `internal/scheduler/scheduler.go`、`internal/scheduler/scheduler_test.go`

- [√] 3.3 管理后台移除限额路由/处理器/模板
  - 文件: `internal/server/app.go`、`internal/admin/server.go`
  - 删除: `internal/admin/channel_limits.go`、`internal/admin/account_limits.go`
  - 模板: `internal/admin/templates/channels.html`、`internal/admin/templates/endpoints.html`

### 4. 文档与验证

- [√] 4.1 知识库文档同步（API/Data/Changelog）
  - 文件: `helloagents/wiki/api.md`、`helloagents/wiki/data.md`、`helloagents/CHANGELOG.md`

- [√] 4.2 运行全量测试
  - 命令: `go test ./...`

---

## 执行备注

| 任务 | 状态 | 备注 |
|------|------|------|
| 全量测试 | completed | `go test ./...` 通过 |
