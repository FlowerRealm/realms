# 任务清单: 订阅套餐后台可配置 + 1d 窗口限额

目录: `helloagents/plan/202601151212_subscription_plans_admin/`

---

## 1. 数据与迁移
- [√] 1.1 `subscription_plans` 增加 `limit_1d_usd_micros`（默认 0 表示不限额）

## 2. Store 与配额
- [√] 2.1 扩展 `SubscriptionPlan` 结构与查询（含 1d 字段）
- [√] 2.2 配额窗口扩展为 `5h/1d/7d/30d`（`<=0` 跳过）

## 3. Web 控制台
- [√] 3.1 `/subscription` 订阅列表增加 1d 展示
- [√] 3.2 不限额窗口展示为“不限”
- [√] 3.3 订阅名称展示同步修正（顶部展示当前订阅名称与有效期；文案一致）

## 4. 管理后台
- [√] 4.1 新增订阅套餐管理页：`/admin/subscriptions`（列表 + 新增）
- [√] 4.2 新增套餐编辑页：`/admin/subscriptions/{plan_id}`（编辑）

## 5. 文档更新
- [√] 5.1 更新 `helloagents/wiki/data.md`
- [√] 5.2 更新 `helloagents/CHANGELOG.md`、`helloagents/history/index.md`

## 6. 测试
- [√] 6.1 `go test ./...`
