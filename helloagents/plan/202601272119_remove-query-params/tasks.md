# 任务清单: remove_query_params

目录: `helloagents/plan/202601272119_remove-query-params/`

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
总任务: 12
已完成: 9
已跳过: 3
完成率: 100% (含跳过)
```

---

## 任务列表

### 1. Web SSR：Query 自动略去
- [√] 1.1 新增中间件 `StripWebQuery`（GET/HEAD 且存在 Query → 302 到无 Query URL，并迁移兼容参数）
- [√] 1.2 接入 Web SSR 路由链（含公开页 `/login`/`/register`）

### 2. Flash/回跳（替换 msg/err/next）
- [√] 2.1 实现一次性 Flash Cookie（notice/error），并在 Web 渲染入口读取/清除
- [√] 2.2 登录回跳：替换 `?next=` 机制为 Cookie（仅保存 Path）

### 3. Web SSR：筛选/分页 Query → Cookie/Path
- [√] 3.1 `/usage`：start/end/limit → cookie；before/after → `/usage/before/{id}` 与 `/usage/after/{id}`；新增 `POST /usage/filter`
- [√] 3.2 `/tickets`：`?status=` → `/tickets/open`、`/tickets/closed`

### 4. Billing：支付回跳去 Query
- [√] 4.1 Stripe success/cancel URL 改为 `/pay/{kind}/{order_id}/success|cancel`，并移除站内 `?msg/err` 跳转

### 5. 文档与验证
- [√] 5.1 更新 `helloagents/CHANGELOG.md` 与方案包文档
- [√] 5.2 运行 `go test ./...` 并确保通过

### 6. 本次范围外（保持现状）
- [-] 6.1 Admin SSR 的 Query 清理/迁移
- [-] 6.2 API `/api/*` 的 Query 清理/迁移
- [-] 6.3 协议端点（OAuth/回调/notify）的 Query 清理/迁移
