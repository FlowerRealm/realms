# 任务清单: ui-icon

> **@status:** completed | 2026-01-22 19:49

目录: `helloagents/archive/2026-01/202601221942_ui-icon/`

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

### 1. 静态资源与路由

- [√] 1.1 将 `realms_icon.svg` 迁移到 `internal/assets/realms_icon.svg` 并通过 `go:embed` 暴露 `assets.RealmsIconSVG()`
  - 验证: `go test ./...` 编译通过

- [√] 1.2 在 `internal/server/app.go` 注册 `/assets/realms_icon.svg` 与 `/favicon.ico` 路由
  - 依赖: 1.1
  - 验证: `GET /assets/realms_icon.svg` 返回 `200 image/svg+xml`；`GET /favicon.ico` 永久重定向到 `/assets/realms_icon.svg`

- [√] 1.3 为静态资源路由补充单元测试 `internal/server/app_test.go`
  - 依赖: 1.2
  - 验证: `go test ./...`

### 2. UI 替换（全站）

- [√] 2.1 更新 Web 控制台模板 `internal/web/templates/base.html`：favicon + Logo 替换（含登录/注册）
  - 验证: 模板引用 `/assets/realms_icon.svg`（favicon + `<img>` Logo）

- [√] 2.2 更新管理后台模板 `internal/admin/templates/base.html`：favicon + Logo 替换
  - 验证: 模板引用 `/assets/realms_icon.svg`（favicon + `<img>` Logo）

### 3. 验证与知识库

- [√] 3.1 运行测试：`go test ./...`
  - 验证: 全部通过

- [√] 3.2 同步知识库：更新 `helloagents/wiki/modules/realms.md` 与 `helloagents/CHANGELOG.md`
  - 验证: 文档已更新且与代码一致

- [√] 3.3 归档方案包到 `helloagents/archive/YYYY-MM/`（status=completed）
  - 验证: `migrate_package.py 202601221942_ui-icon --status completed` 成功；`archive/_index.md` 已更新

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
