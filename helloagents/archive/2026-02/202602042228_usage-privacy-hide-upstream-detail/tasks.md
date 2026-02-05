# 任务清单: usage-privacy-hide-upstream-detail

> **@status:** completed | 2026-02-04 22:30

目录: `helloagents/plan/202602042228_usage-privacy-hide-upstream-detail/`

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
总任务: 5
已完成: 5
完成率: 100%
```

---

## 任务列表

### 1. Router/API（用户侧裁剪）

- [√] 1.1 用户侧用量列表不返回上游端点/凭据标识
  - 文件: `router/usage_api_routes.go`
  - 验证: `go test ./...` + Playwright 断言 `/api/usage/events` 不包含 `upstream_*` 标识

- [√] 1.2 用户侧用量明细不返回上游请求/响应明细
  - 文件: `router/usage_api_routes.go`
  - 验证: Playwright 断言 `/api/usage/events/:id/detail` 不包含 `upstream_request_body/upstream_response_body`

### 2. Web/Frontend（用户侧展示）

- [√] 2.1 用量页展开明细时，对“上游明细缺失”显示“仅管理员可查看”
  - 文件: `web/src/pages/UsagePage.tsx`
  - 验证: Playwright 断言 UI 中出现“仅管理员可查看”，且不出现渠道名

### 3. Web/E2E（失败请求回归覆盖）

- [√] 3.1 fake upstream 支持构造 400 响应并在响应体中包含渠道名字段
  - 文件: `cmd/realms-e2e/main.go`
  - 验证: Playwright 触发 `input="__pw_fail__"` 返回 400

- [√] 3.2 增强用量页隐私回归：覆盖失败请求明细（available=true）
  - 文件: `web/e2e/usage.spec.ts`
  - 验证: `npm --prefix web run build && npm --prefix web run test:e2e:ci -- e2e/usage.spec.ts`

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 1.1~3.2 | completed | 已通过 `go test ./...` 与 Playwright 用量用例验证 |
