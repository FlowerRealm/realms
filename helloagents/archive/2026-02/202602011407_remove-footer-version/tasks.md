# 任务清单: remove-footer-version

> **@status:** completed | 2026-02-01 14:58

目录: `helloagents/archive/2026-02/202602011407_remove-footer-version/`

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
总任务: 10
已完成: 10
完成率: 100%
```

---

## 任务列表

### 1. 前端（Web SPA）

- [√] 1.1 移除页脚版本号展示与拉取逻辑（`web/src/layout/ProjectFooter.tsx`）
  - 验证: `npm --prefix web run build`；手动打开 public/app/admin 页面确认页脚无版本号且无 `/api/version` 请求

### 2. 后端（Router / Server）

- [√] 2.1 删除系统路由 `/api/version`（`router/system_routes.go`）
  - 验证: 启动服务后 `curl -i http://localhost:8080/api/version` 返回 404

- [√] 2.2 移除 `router.Options.Version` 字段并更新所有构造点（`router/options.go`，以及所有 `router.Options{...}` 初始化处）
  - 验证: `go test ./...`

- [√] 2.3 删除 `internal/server/app.go` 中 `handleVersion` 及注入点（`router.SetRouter(... Options{ Version: ... })`）
  - 依赖: 2.2
  - 验证: `go test ./...`

### 3. 文档与知识库同步

- [√] 3.1 删除/改写所有 `/api/version` 相关描述（README / docs / helloagents KB）
  - 目标文件:
    - `README.md`
    - `docs/api.md`
    - `docs/index.md`
    - `docs/versioning.md`
    - `helloagents/wiki/api.md`
    - `helloagents/wiki/modules/realms.md`
    - `helloagents/modules/web_spa.md`
  - 验证: `rg \"/api/version\" -S .` 不再出现（允许在历史归档/方案包中出现）

### 4. 回归验证

- [√] 4.1 运行后端测试：`go test ./...`

- [√] 4.2 运行前端构建：`npm --prefix web run build`

- [√] 4.3 （可选但推荐）运行 Web E2E：`npm --prefix web run test:e2e:ci`

### 5. 交付与归档

- [√] 5.1 更新 `helloagents/CHANGELOG.md`（记录“移除页脚版本号展示 / 移除 /api/version”并附方案链接）

- [√] 5.2 迁移方案包至 `helloagents/archive/2026-02/202602011407_remove-footer-version/` 并更新 `helloagents/archive/_index.md`

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
