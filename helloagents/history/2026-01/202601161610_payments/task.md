# 任务清单: 支付与按量计费（充值/订阅）

目录: `helloagents/plan/202601161610_payments/`

---

## 1. 数据层（余额/充值/订单）
- [√] 1.1 新增迁移：`user_balances` 与 `topup_orders` 表
- [√] 1.2 补齐 `subscription_orders` 的“支付后生效”实现（保留订单记录，不再删除）
- [√] 1.3 实现 Store 方法：余额查询/扣减/返还、充值订单创建/列表/入账、订阅订单入账

## 2. 配置与后台设置
- [√] 2.1 扩展 `config.yaml`：新增 `payment` 与 `billing` 默认配置结构（默认关闭）
- [√] 2.2 扩展 `app_settings`：新增支付与按量计费相关 key（支持 UI 覆盖）
- [√] 2.3 扩展 `/admin/settings` 页面：分别配置 EPay 与 Stripe + 按量计费参数（敏感字段不回显）

## 3. 支付渠道与回调
- [√] 3.1 实现 Stripe Checkout Session 创建 + Webhook 回调入账（验签、金额校验、幂等）
- [√] 3.2 实现 EPay 支付跳转 URL 生成 + notify 回调入账（验签、金额校验、幂等）

## 4. Web 控制台（用户侧）
- [√] 4.1 新增 `/topup` 充值页面：展示余额、展示充值订单、创建充值订单入口
- [√] 4.2 调整订阅购买流程：下单后跳转到支付页（`/pay/subscription/{order_id}`）
- [√] 4.3 新增支付页 `/pay/{kind}/{order_id}`：用户点击支付后跳转到支付渠道页面

## 5. 按量计费（Quota）
- [√] 5.1 新增按量计费 QuotaProvider：余额预留→结算→作废/过期返还（并发安全）
- [√] 5.2 改造 App 使用“订阅优先 + 余额兜底”的 QuotaProvider
- [√] 5.3 用量清理任务支持“过期预留返还余额”

## 6. 安全检查
- [√] 6.1 执行安全检查（按G9：输入校验、敏感信息处理、权限控制、支付回调验签与幂等、避免并发透支）

## 7. 文档更新（知识库）
- [√] 7.1 更新 `helloagents/wiki/api.md`：新增充值/支付/回调路由说明
- [√] 7.2 更新 `helloagents/wiki/data.md`：新增 `user_balances` 与 `topup_orders`
- [√] 7.3 更新 `helloagents/wiki/modules/realms.md`：记录模块变更历史
- [√] 7.4 更新 `helloagents/CHANGELOG.md`：记录新增支付与按量计费能力

## 8. 测试
- [√] 8.1 执行 `go test ./...`
