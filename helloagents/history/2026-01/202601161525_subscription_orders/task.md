# 任务清单: 订阅订单（下单/支付/生效）

目录: `helloagents/plan/202601161525_subscription_orders/`

---

## 1. Store / 数据层
- [√] 1.1 新增迁移：创建 `subscription_orders` 表
- [√] 1.2 新增模型：`store.SubscriptionOrder`
- [√] 1.3 实现订单 API：下单、列表、支付生效、批准生效（事务 + 幂等）

## 2. Web 控制台（用户侧）
- [√] 2.1 调整 `POST /subscription/purchase`：从“直接生效”改为“创建订单”
- [√] 2.2 订阅页展示订单列表与成功提示（Notice）

## 3. 管理后台（仅 root）
- [√] 3.1 新增 `/admin/orders` 订单列表页
- [√] 3.2 新增订单操作：标记支付并生效 / 批准生效
- [√] 3.3 补齐路由注册与侧边栏入口

## 4. 安全检查
- [√] 4.1 校验输入与权限：仅 root 可操作订单生效；避免重复创建订阅

## 5. 文档更新（知识库）
- [√] 5.1 更新 `helloagents/wiki/data.md`：补充 `subscription_orders`
- [√] 5.2 更新 `helloagents/wiki/api.md`：更新购买语义与管理入口
- [√] 5.3 更新 `helloagents/CHANGELOG.md`：记录新增订单能力

## 6. 测试
- [√] 6.1 执行 `go test ./...`
