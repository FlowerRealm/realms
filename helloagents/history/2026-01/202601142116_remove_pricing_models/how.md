# 技术设计: 取消独立定价规则（pricing_models），定价与模型绑定

## 技术方案

### 实现要点

1. **路由与 UI**
   - 移除 `internal/server/app.go` 中 `/admin/pricing-models` 路由注册。
   - 移除 `internal/admin/templates/base.html` 中“定价规则”导航入口。

2. **Admin 代码与模板**
   - 删除 `internal/admin/pricing_models.go` 与 `internal/admin/templates/pricing_models.html`。
   - `internal/admin/server.go` 的 `templateData` 移除 `PricingModels/PricingModel` 字段，避免遗留概念传播。

3. **Store 层**
   - 删除 `internal/store/pricing.go` 与 `store.PricingModel` 结构体（不再暴露独立定价规则的 CRUD）。

4. **计费**
   - `internal/quota/quota.go` 的成本计算仅查询 `managed_models`，不再回退 `pricing_models` pattern 匹配。
   - 若模型不存在（不应发生），返回错误用于暴露不一致问题（上游调用忽略错误时仍会保留 reserved 直到过期）。

5. **数据库**
   - 新增迁移 `0012_drop_pricing_models.sql`：`DROP TABLE IF EXISTS pricing_models;`。

## 测试与部署

- **测试:** `go test ./...`
- **部署:** 发布后自动执行迁移；如需调整单价，统一在 `/admin/models` 配置。
