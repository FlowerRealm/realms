# 任务清单: 订阅购买与额度限制

目录: `helloagents/plan/202601141319_subscription_purchase/`

---

## 1. 数据与迁移
- [√] 1.1 新增 `subscription_plans`/`user_subscriptions` 迁移，并插入默认套餐
- [√] 1.2 插入 `pricing_models` 全局兜底定价（priority=-100）

## 2. 配额实现
- [√] 2.1 实现 `SubscriptionProvider`：未订阅/超限拒绝，按 5h/7d/30d 滚动窗口判断
- [√] 2.2 预留阶段写入 `reserved_usd_micros`，结算阶段无 usage 时回落到 reserved（防 SSE 绕过）

## 3. Web 控制台
- [√] 3.1 `/subscription` 展示当前订阅、套餐列表与购买按钮
- [√] 3.2 `/subscription/purchase` 购买逻辑接入 Store（每次购买新增订阅记录）

## 4. 文档更新
- [√] 4.1 更新 `helloagents/wiki/api.md`、`helloagents/wiki/data.md`、`helloagents/wiki/modules/codex.md`
- [√] 4.2 更新 `helloagents/CHANGELOG.md`、`helloagents/history/index.md`

## 5. 测试
- [√] 5.1 `go test ./...`
