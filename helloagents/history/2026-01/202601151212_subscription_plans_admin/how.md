# 技术设计: 订阅套餐后台可配置 + 1d 窗口限额

## 设计原则

- **保持现有语义不变**：额度仍按 `usd_micros` 计量，成本估算依赖模型定价（`managed_models`）。
- **可选窗口不引入 NULL**：延续现有实现，`limit <= 0` 视为“该窗口不限额”，避免全链路改造为 nullable。
- **不破坏既有购买逻辑**：用户侧购买仍按 `plan_code` 创建 `user_subscriptions`，不影响存量订阅记录。

## 数据模型

迁移：`internal/store/migrations/0017_subscription_plans_limit_1d.sql`

- `subscription_plans.limit_1d_usd_micros BIGINT NOT NULL DEFAULT 0`
  - `<=0` 表示该窗口不限额

## 配额实现

位置：`internal/quota/subscription.go`

- 将窗口列表从 `5h/7d/30d` 扩展为 `5h/1d/7d/30d`
- 对每个有效订阅逐窗口判断：`committed + reserved(active) + this_reserve <= limit`
- `limit <= 0` 的窗口直接跳过

## 控制台实现

### 管理后台

- 新增页面：`GET /admin/subscriptions`
  - 列表 + 新增（Modal）
- 新增页面：`GET /admin/subscriptions/{plan_id}`
  - 编辑
- 新增/编辑字段：
  - `code/name/price_cny/duration_days/status`
  - `limit_5h/limit_1d/limit_7d/limit_30d`（留空=不限）

### 用户控制台

- `/subscription` 的订阅列表增加 `1d` 展示
- `limit<=0` 的窗口展示为“不限”
- 页面顶部展示“当前订阅名称 + 有效期”，并统一“订阅”用词（避免与“套餐”混用）

## 验证

- `go test ./...`
