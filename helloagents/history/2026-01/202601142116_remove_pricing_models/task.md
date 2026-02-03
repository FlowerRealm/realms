# 任务清单: 取消独立定价规则（pricing_models），定价与模型绑定

目录: `helloagents/plan/202601142116_remove_pricing_models/`

---

## 1. 路由与 UI
- [√] 1.1 更新 `internal/server/app.go`：移除 `/admin/pricing-models` 路由注册
- [√] 1.2 更新 `internal/admin/templates/base.html`：移除“定价规则”导航入口

## 2. Admin 与 Store 清理
- [√] 2.1 删除 `internal/admin/pricing_models.go` 与 `internal/admin/templates/pricing_models.html`
- [√] 2.2 更新 `internal/admin/server.go`：移除 `templateData` 中的 `PricingModels/PricingModel`
- [√] 2.3 删除 `internal/store/pricing.go` 与 `store.PricingModel`（如有）

## 3. 计费逻辑
- [√] 3.1 更新 `internal/quota/quota.go`：成本计算仅使用 `managed_models` 定价，不再回退 `pricing_models`

## 4. 数据库迁移
- [√] 4.1 新增 `internal/store/migrations/0012_drop_pricing_models.sql`：移除 `pricing_models` 表

## 5. 文档更新
- [√] 5.1 更新 `helloagents/wiki/api.md`：移除 `/admin/pricing-models`
- [√] 5.2 更新 `helloagents/wiki/data.md`：移除 `pricing_models` 描述
- [√] 5.3 更新 `helloagents/CHANGELOG.md`：记录定价收敛为模型定价并下线 `/admin/pricing-models`

## 6. 测试
- [√] 6.1 运行 `go test ./...`
