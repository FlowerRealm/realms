# 技术设计: 支付页支持关闭订单（订阅/充值）

## 技术方案

### 核心技术
- Go `net/http` 路由（SSR）
- MySQL 事务 + `SELECT ... FOR UPDATE`
- Cookie Session + CSRF

### 实现要点
- Web 支付页（`internal/web/templates/pay.html`）在订单状态为“待支付”时展示“关闭订单”按钮（无需二次确认弹窗）。
- 新增 SSR 路由：`POST /pay/{kind}/{order_id}/cancel`，由 `internal/web/server.go` 实现：
  - 校验用户会话与 CSRF
  - 按 `kind` 调用 store 层的按用户取消订单方法
  - PRG 跳转回支付页并提示结果
- Store 层新增事务方法：
  - `CancelSubscriptionOrderByUser(ctx, userID, orderID)`
  - `CancelTopupOrderByUser(ctx, userID, orderID)`
  - 仅允许对“待支付”订单更新为“已取消”，其他状态返回错误
- 支付回调处理：
  - 当回调检测到订单已取消时，不自动入账/生效；
  - 尽量把支付元信息写入订单（`paid_at/paid_method/paid_ref/paid_channel_id`），并返回成功，避免回调反复重试。

## API 设计

### [POST] /pay/{kind}/{order_id}/cancel
- **认证:** Cookie 会话 + CSRF
- **语义:** 关闭待支付订单（subscription/topup），订单状态标记为“已取消”
- **返回:** 302 跳转回支付页（`/pay/{kind}/{order_id}`）展示提示信息

## 安全与性能
- **安全:**
  - 严格限制仅“待支付”可取消
  - 订单归属校验：store 方法以 `id AND user_id` 锁定行，避免越权
  - CSRF 校验与 PRG 跳转避免重复提交
- **性能:** 单行锁 + 简单更新；仅影响单个订单记录

## 测试与部署
- **测试:** 执行 `go test ./...`；重点关注构建通过与 webhook 路径逻辑无编译回归
- **部署:** 无迁移；滚动发布即可

