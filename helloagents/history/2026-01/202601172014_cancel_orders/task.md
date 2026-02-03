# 任务清单: 支付页支持关闭订单（订阅/充值）

目录: `helloagents/history/2026-01/202601172014_cancel_orders/`
---

## 1. Web 控制台（用户）
- [√] 1.1 在支付页（`internal/web/templates/pay.html`）为“待支付”订单增加“关闭订单”入口（无二次确认弹窗）
- [√] 1.2 新增路由 `POST /pay/{kind}/{order_id}/cancel`（`internal/server/app.go` + `internal/web/server.go`）并实现 PRG 跳转与提示

## 2. Store（订单状态）
- [√] 2.1 实现 `CancelSubscriptionOrderByUser` 与 `CancelTopupOrderByUser`（仅允许待支付 → 已取消）
- [√] 2.2 支付回调遇到“订单已取消”时，记录支付元信息但不入账/不生效，且避免回调重试放大

## 3. 文档更新
- [√] 3.1 更新 `helloagents/wiki/api.md` 与 `helloagents/wiki/modules/realms.md` 补充关闭订单接口与语义
- [√] 3.2 更新 `helloagents/CHANGELOG.md` 记录变更

## 4. 安全检查
- [√] 4.1 执行支付相关安全检查（权限/CSRF/事务幂等/回调取消态处理）

## 5. 测试
- [√] 5.1 执行 `go test ./...`
