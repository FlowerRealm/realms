# 变更提案: 取消独立定价规则（pricing_models），定价与模型绑定

## 需求背景

当前系统已经支持在 `managed_models` 维度配置模型的 input/output/cache 三类价格，并在计费侧按该价格结算。

独立的 `pricing_models`（pattern 定价规则）会带来两类问题：
1. **配置入口重复**：模型已经是 SSOT，额外的定价规则页面会造成运维困惑与不一致风险。
2. **口径不清晰**：pattern 匹配会引入隐式覆盖关系，不利于“按模型明确计费”的目标。

因此需要下线 `pricing_models` 的独立管理与依赖，收敛为“定价与模型绑定”的单一口径。

## 变更内容

1. 下线 `/admin/pricing-models` 页面与相关路由/handler/template。
2. 计费不再回退 `pricing_models` 规则，仅使用 `managed_models` 的定价字段。
3. 数据库迁移中移除 `pricing_models` 表（保留历史迁移文件不改动，通过新迁移删除）。

## 影响范围

- **模块:** admin / quota / store / migrations / docs
- **文件:**
  - `internal/server/app.go`
  - `internal/admin/templates/base.html`
  - `internal/admin/server.go`
  - `internal/quota/quota.go`
  - `internal/store/migrations/0012_drop_pricing_models.sql`
  - `helloagents/wiki/api.md`
  - `helloagents/wiki/data.md`
  - `helloagents/CHANGELOG.md`

## 风险评估

- **风险:** 依赖 `/admin/pricing-models` 的运维流程将失效。
- **缓解:** 定价入口统一在 `/admin/models`；上线前确认各模型价格已配置（新列有默认值作为兜底）。
