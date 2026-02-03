# 任务清单: 订阅套餐强制删除（含级联清理）

目录: `helloagents/plan/202601162035_subscription_plan_force_delete/`

---

## 1. Store：强制删除套餐（允许被引用）
- [√] 1.1 删除套餐时级联清理 `user_subscriptions/subscription_orders`
- [√] 1.2 可选：解绑 `usage_events.subscription_id`（避免引用已删除订阅）

## 2. Admin：删除接口调整
- [√] 2.1 删除 handler 改为强制删除，并返回清理摘要
- [√] 2.2 更新前端确认提示（明确会影响订阅/订单）

## 3. 文档更新
- [√] 3.1 更新 `helloagents/wiki/api.md` 描述强制删除语义
- [√] 3.2 更新 `helloagents/wiki/modules/realms.md` 记录强制删除行为
- [√] 3.3 更新 `helloagents/CHANGELOG.md` 与 `helloagents/history/index.md`

## 4. 测试
- [√] 4.1 运行 `go test ./...`
