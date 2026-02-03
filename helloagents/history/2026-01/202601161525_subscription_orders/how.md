# 技术设计: 订阅订单（下单/支付/生效）

## 技术方案

### 核心技术
- **后端:** Go 1.22（`net/http` + SSR 模板）
- **数据库:** MySQL（内置迁移：`internal/store/migrations/*.sql`）

### 实现要点
- 新增 `subscription_orders` 表作为订阅购买的 SSOT（订单 → 订阅）。
- 下单仅创建订单，不再直接写入 `user_subscriptions`。
- “支付/批准”触发订单激活：在同一事务内写入订单元信息并创建 `user_subscriptions`（幂等）。
- 管理后台提供订单列表与操作入口（仅 `root`）。

## 数据模型

新增表：`subscription_orders`

- `user_id/plan_id/amount_cny_fen`：购买快照（金额来自套餐当前价格）
- `status`：0=待支付，1=已生效，2=已取消（预留）
- `paid_at/paid_method/paid_ref`：支付信息（best-effort）
- `approved_at/approved_by`：管理员批准信息（兜底）
- `subscription_id`：关联创建出的 `user_subscriptions.id`（用于幂等与追溯）
- `note`：备注（可选）

## 订单状态机

最小可用状态流转：

1. `待支付`（创建订单）
2. `已生效`（支付确认自动生效，或管理员批准生效）

说明：当前实现不强制区分“已支付未生效”状态；如未来需要人工审核再生效，可扩展状态枚举。

## API 设计（SSR）

### 用户侧
- `POST /subscription/purchase`
  - 输入：`plan_id`
  - 输出：创建订单并重定向回 `/subscription?msg=...`

### 管理侧（仅 root）
- `GET /admin/orders`：订单列表（最近 200 条）
- `POST /admin/orders/{order_id}/paid`：标记支付并生效（写 `paid_at`，自动创建订阅）
- `POST /admin/orders/{order_id}/approve`：批准并生效（写 `approved_*`，自动创建订阅）

## 安全与性能

- **安全:**
  - 管理操作仅限 `root`，并使用 CSRF 中间件
  - 订单激活使用 `SELECT ... FOR UPDATE` 锁定订单行，避免并发重复创建订阅
- **性能:**
  - `subscription_orders` 增加按 `user_id/id` 与 `status/id` 索引，满足列表与筛选

## 测试与部署

- **测试:** `go test ./...`（确保构建与既有测试通过）
- **部署:** 启动时自动执行新迁移文件，升级即生效

