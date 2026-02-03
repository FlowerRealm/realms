# 变更提案: remove_query_params

## 元信息
```yaml
类型: 变更/重构
方案类型: implementation
优先级: P0
状态: ✅已完成
创建: 2026-01-27
完成: 2026-01-27
```

---

## 1. 需求

### 背景
当前项目在多处使用 URL Query 参数（`?k=v`）来承载状态与交互，例如：
- 页面筛选/分页：`/usage?start=YYYY-MM-DD&end=YYYY-MM-DD&limit=50`
- 登录回跳：`/login?next=/some/path`
- 提示信息：`?msg=...` / `?err=...`
- 管理后台 UI 状态：`?open_channel_settings=...`、`?edit=...`
- 第三方交互：OAuth callback（`code/state`）、EPay 通知回调参数等

需求是：**Web 控制台（SSR）的 URL 不再保留 Query 参数**：对 GET/HEAD 若含 Query，自动 302 到无 Query 的等价 URL，并做必要的兼容迁移（`msg/err/next`、`/usage`、`/tickets`、支付回跳等）。API/协议端点不做“全站禁用 Query”。

### 目标
- Web SSR：GET/HEAD 若含 Query，自动 302 到无 Query 的等价 URL（浏览器地址栏不保留 `?k=v`）。
- Web SSR：站内跳转不再生成 `?msg/?err/?next` 等 Query；提示与回跳迁移到 Cookie（flash/next）。
- 用量页默认展示：保持现有默认（用户选择：A）——**UTC 的“今天(00:00) ~ 现在”**，并使用现有默认 limit。
- 兼容迁移：对历史 Query（`msg/err/next`、`/usage`、`/tickets`）自动迁移到 cookie/path。

### 非目标（显式声明）
- 管理后台（`/admin`）与 Token/API 接口的 Query 清理/迁移。
- 协议端点（如 `/oauth/authorize`、OAuth callback、支付 notify）不做 Query 禁用；必要时在中间件中显式排除。

### 约束条件
```yaml
兼容性约束: 属于 URL 行为变更；历史含 Query 的 SSR 请求将被 302 迁移到无 Query URL
安全约束: 不引入新的开放重定向风险；所有“next/return_to” 必须继续做路径白名单/净化
行为约束: 站内跳转与错误提示需有替代承载方式（不能退化为“无提示/死路由”）
```

### 验收标准
- [√] Web SSR 自动略去 Query：`GET /usage?start=...` 302 到 `/usage` / `/usage/before/{id}`，并保留筛选/分页语义
- [√] 登录回跳不再使用 `?next=`：未登录访问受保护页会写 `rlm_next` cookie（仅 path）并跳转 `/login`
- [√] 站内不再生成 `?msg/?err/?next`、`before_id/after_id`、`status=open|closed` 等 Query（Web 控制台）
- [√] `go test ./...` 通过

---

## 2. 方案

### 总体策略
1) **新增 Web SSR 中间件：StripWebQuery**
   - 仅对 GET/HEAD 生效：若存在 query 则 302 跳转到无 query 的等价 URL。
   - 兼容迁移：`msg/err` → flash cookie；`next` → `rlm_next` cookie（仅 path）；`/usage` 筛选 → cookie、分页 → path；`/tickets` 筛选 → path。
   - 排除协议端点：`/oauth/authorize`（Query 必需）。

2) **站内状态迁移：Query → Cookie/Path**
   - `msg/err`：一次性 Flash Cookie（读取后立即清除）替代，映射到 `TemplateData.Notice/Error`。
   - `next`：短期 Cookie 保存回跳 path（不保留 Query）。
   - `/usage`：新增 `POST /usage/filter` 写 cookie；分页使用 `/usage/before/{id}` 与 `/usage/after/{id}`。
   - `/tickets`：新增 `/tickets/open`、`/tickets/closed`。

3) **支付回跳去 Query**
   - Stripe success/cancel URL 改为 `/pay/{kind}/{order_id}/success|cancel`，回跳后写 flash 并重定向到 `/pay/{kind}/{order_id}`。

### 影响范围（本次实现）
```yaml
本次改动（Web 控制台 SSR）:
  - internal/middleware/strip_web_query.go: SSR Query → 302 + 兼容迁移
  - internal/middleware/flash.go: flash/next/usage filter cookies
  - internal/middleware/session_auth.go: next → cookie（仅 path）
  - internal/server/app.go: 接入中间件与新增路由
  - internal/web/server.go: 移除 Query 读取/拼接；新增 usage/filter、pay return handlers
  - internal/web/tickets.go: tickets 过滤改为 path
  - internal/web/templates/usage.html: 筛选改 POST、分页改 path
  - internal/web/templates/tickets.html: status 过滤改为 path

范围外（保持现状）:
  - internal/web/api_usage.go: `/api/usage/*` 仍使用 Query
  - internal/web/oauth_apps.go: `/oauth/authorize` 仍依赖 Query（已在中间件中排除）
  - internal/admin/**: 管理后台 Query 未迁移
  - internal/server/payment_webhooks.go / internal/codexoauth/flow.go: 协议回调仍依赖 Query
```

### 风险评估
| 风险 | 等级 | 说明 | 应对 |
|------|------|------|------|
| 协议端点误被 strip | 中 | OAuth authorize 等若被重定向会破坏协议 | `StripWebQuery` 显式排除 `/oauth/authorize`，且仅挂接在 Web SSR 链路 |
| 站内信息提示丢失 | 低 | 之前依赖 `?msg/?err` | 迁移到 Flash Cookie，统一替换 |
| 回跳引入开放重定向 | 中 | `next` 从 Query/Cookie 迁移后仍需净化 | `SetNextPathCookie` 仅保存站内 path；登录页继续 `sanitizeNextPath` |
| URL 行为变化导致书签失效 | 低 | 旧带 Query URL 会改变 | `StripWebQuery` 提供 302 兼容迁移到新路径 |

---

## 3. 核心场景

### 场景 1：Web SSR 带 Query 自动跳转到无 Query
**条件**：访问 `/login?next=/dashboard&msg=...`、`/usage?start=...&end=...&limit=...`、`/tickets?status=open`
**行为**：服务端写入兼容 cookie（flash/next/usage filter），并 302 到无 Query 的等价 URL
**结果**：浏览器地址栏不保留 Query，页面语义保持一致

### 场景 2：登录回跳不再依赖 `?next=`
**条件**：未登录访问受保护页（例如 `/usage`）
**行为**：鉴权中间件写入 `rlm_next` Cookie（短期，仅 path），重定向到 `/login`；登录成功后按 Cookie 回跳
**结果**：无 Query 仍能正确回跳

### 场景 3：用量页默认展示（A）
**条件**：访问 `/usage`（无 Query）
**行为**：展示 UTC 今日 00:00 ~ 当前时间，limit 使用默认值
**结果**：与现有默认一致

---

## 4. 技术决策（待确认）

### remove_query_params#D001: SSR Query 处理方式
- 已确定：对 Web SSR 的 GET/HEAD 采用 `302 Found` 跳转到无 Query URL（并做兼容迁移）。

### remove_query_params#D002: 协议端点处置
- 已确定：不做全站 Query 禁用；对 `/oauth/authorize` 显式排除，保留 Query 语义。
