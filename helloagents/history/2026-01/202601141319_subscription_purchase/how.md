# 技术设计: 订阅购买与额度限制

## 技术方案

### 核心技术
- Go（`net/http` SSR）
- MySQL 迁移（内置 `internal/store/migrations/*.sql`）
- 滚动窗口限额：基于 `usage_events` 的 `committed_usd_micros` + `reserved_usd_micros`

### 实现要点
- **订阅数据模型**
  - `subscription_plans`：套餐定义（价格、3 个窗口的 usd_micros 限额、duration）
  - `user_subscriptions`：用户订阅（同一用户可并发多条记录；记录 start/end；额度按有效订阅并行叠加）
- **购买逻辑**
  - 每次购买：创建 `start_at=now, end_at=now+duration`（不再通过 end_at 叠加续费）
- **配额预留与并发保护**
  - 预留阶段使用请求中的 `max_tokens/max_output_tokens` 估算“最大输出成本”，写入 `reserved_usd_micros`
  - 窗口判断基于：`committed + reserved(active) + this_reserve <= limit`
  - 结算阶段：若拿不到 usage tokens（SSE），使用 `reserved_usd_micros` 作为结算成本，避免流式绕过
- **成本兜底**
  - 迁移插入 `pricing_models(scope=global, pattern='*', priority=-100)`，确保没有更具体定价时也能得到非 0 成本（可按需在管理面更精细配置覆盖）

## API 设计

### [POST] /subscription/purchase
- **用途:** Web 控制台购买套餐（Cookie 会话 + CSRF）
- **请求:** `plan=<code>`
- **响应:** 302 重定向回 `/subscription`

### 数据面错误提示
- 未订阅：`429 订阅未激活`
- 超限：`429 订阅额度不足`

## 数据模型

迁移文件：`internal/store/migrations/0002_subscriptions.sql`

- `subscription_plans`
- `user_subscriptions`
- `pricing_models` 插入兜底行

## 安全与性能
- **安全:** 购买接口受 SessionAuth + CSRF 保护；订阅查询按 user/group 隔离。
- **性能:** 窗口汇总使用单次聚合查询（`SUM(CASE WHEN ...)`），并仅在预留阶段执行 3 次窗口检查（5h/7d/30d）。

## 测试与部署
- `go test ./...`
- 运行服务后通过 `/subscription` 购买，再调用 `/v1/responses` 验证限额行为。
