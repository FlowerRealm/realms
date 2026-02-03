# 任务清单: 订阅套餐删除（管理后台）

目录: `helloagents/plan/202601162005_subscription_plan_delete/`

---

## 1. Store：订阅套餐删除方法
- [√] 1.1 增加 `DeleteSubscriptionPlan`：仅在无订阅/订单引用时允许删除
- [√] 1.2 在被引用时返回可提示的错误（建议禁用而非删除）

## 2. Admin：删除路由与处理器
- [√] 2.1 增加 `POST /admin/subscriptions/{plan_id}/delete`
- [√] 2.2 增加 `DeleteSubscriptionPlan` handler（兼容 AJAX/非 AJAX）

## 3. Admin UI：订阅套餐列表增加删除入口
- [√] 3.1 `/admin/subscriptions` 列表页增加“删除”按钮（含确认弹窗与 CSRF）

## 4. 文档更新
- [√] 4.1 更新 `helloagents/wiki/modules/realms.md` 记录该行为
- [√] 4.2 更新 `helloagents/CHANGELOG.md` 增加变更记录
- [√] 4.3 更新 `helloagents/history/index.md` 增加变更索引

## 5. 测试
- [√] 5.1 运行 `go test ./...`
