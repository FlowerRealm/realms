# 任务清单: 支付渠道化（按渠道配置 EPay/Stripe）

目录: `helloagents/plan/202601161926_payment_channels/`

---

## 1. 数据层（支付渠道模型）
- [√] 1.1 新增迁移：创建 `payment_channels` 表，验证 why.md#需求-支付渠道独立配置-场景-管理员创建并维护支付渠道
- [√] 1.2 新增迁移：为 `subscription_orders`/`topup_orders` 增加 `paid_channel_id`（可为空），验证 why.md#需求-支付回调按渠道验签与幂等入账-场景-stripe-webhook
- [√] 1.3 新增 Store 封装：支付渠道 CRUD/列表（按 type/status/priority），验证 why.md#需求-支付渠道独立配置-场景-管理员创建并维护支付渠道

## 2. 管理后台（支付渠道配置）
- [√] 2.1 新增后台页面：`/admin/payment-channels`（列表/创建/更新/删除），验证 why.md#需求-支付渠道独立配置-场景-管理员创建并维护支付渠道
- [√] 2.2 敏感字段处理：密钥不回显；留空表示保持不变；提供“回调 URL”提示，验证 why.md#需求-支付渠道独立配置-场景-管理员创建并维护支付渠道

## 3. 用户侧支付页（选渠道）
- [√] 3.1 支付页展示可用支付渠道列表（含多个 stripe/epay），验证 why.md#需求-支付渠道独立配置-场景-用户在支付页选择渠道发起支付
- [√] 3.2 `StartPayment` 支持 `payment_channel_id` 发起支付；保留旧 `method` 兼容，验证 why.md#需求-旧配置兼容可选-场景-未配置支付渠道时继续可用

## 4. 支付回调（按渠道验签）
- [√] 4.1 新增路由：`POST /api/pay/stripe/webhook/{payment_channel_id}` 与 `GET /api/pay/epay/notify/{payment_channel_id}`，验证 why.md#需求-支付回调按渠道验签与幂等入账-场景-stripe-webhook
- [√] 4.2 回调实现：按 `payment_channel_id` 读取配置并验签，幂等入账/生效并写入 `paid_channel_id`，验证 why.md#需求-支付回调按渠道验签与幂等入账-场景-epay-notify
- [√] 4.3 旧回调兼容：保留现有 `/api/pay/stripe/webhook` 与 `/api/pay/epay/notify` 行为（当未配置支付渠道时使用），验证 why.md#需求-旧配置兼容可选-场景-未配置支付渠道时继续可用

## 5. 安全检查
- [√] 5.1 执行安全检查（按G9：输入校验、敏感信息处理、权限控制、支付回调验签与幂等、避免重复入账）

## 6. 文档更新（知识库）
- [√] 6.1 更新 `helloagents/wiki/api.md`：补充支付渠道参数与新回调路由
- [√] 6.2 更新 `helloagents/wiki/data.md`：补充 `payment_channels` 与订单 `paid_channel_id`
- [√] 6.3 更新 `helloagents/wiki/modules/realms.md` 与 `helloagents/CHANGELOG.md`：记录本次变更

## 7. 测试
- [√] 7.1 新增/补齐测试：支付渠道选择、回调路由参数解析、幂等入账（最小覆盖）
- [√] 7.2 执行 `go test ./...`
