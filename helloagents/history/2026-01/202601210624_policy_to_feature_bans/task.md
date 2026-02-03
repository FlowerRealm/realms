# 任务清单: 移除 policy_*，以功能禁用表达数据面语义

目录: `helloagents/history/2026-01/202601210624_policy_to_feature_bans/`

---

## 1. Store（开关与生效逻辑）
- [√] 1.1 在 `internal/store/app_settings.go` 新增 `feature_disable_models` key，并移除 `policy_*` 与 legacy models keys 常量，验证 why.md#requirement-migrate-legacy-policy-and-model-flags
- [√] 1.2 在 `internal/store/features.go` 与 `internal/store/feature_gate_effective.go` 中引入 `ModelsDisabled`，删除 web/admin models 分裂字段，验证 why.md#requirement-models-disable-model-passthrough
- [√] 1.3 删除 `internal/store/policies.go` 与相关测试，并清理引用点，验证 why.md#requirement-billing-disable-free-mode

## 2. 管理后台（系统设置页）
- [√] 2.1 在 `internal/admin/templates/settings.html` 删除“数据面策略（Policy）”卡片，仅保留“功能禁用”卡片，验证 why.md#requirement-billing-disable-free-mode
- [√] 2.2 在 `internal/admin/server.go` 移除 policy 相关 templateData/读写逻辑，并在功能禁用中增加“模型（全禁）”条目（语义提示：模型穿透），验证 why.md#requirement-models-disable-model-passthrough

## 3. 路由与模板（禁用=全禁）
- [√] 3.1 在 `internal/server/app.go` 将 `/models`、`/admin/models*`、`/v1/models` 统一绑定到 `feature_disable_models`，并删除 legacy key 绑定，验证 why.md#requirement-models-disable-model-passthrough
- [√] 3.2 在 `internal/web/templates/base.html` 与 `internal/admin/templates/base.html` 使用新的 `Features.ModelsDisabled` 控制侧边栏入口，验证 why.md#requirement-models-disable-model-passthrough

## 4. 数据面语义（替代 policy）
- [√] 4.1 在 `internal/quota/*` 中用 `feature_disable_billing` 决定 free/normal provider（`self_mode` 仍强制 free），验证 why.md#requirement-billing-disable-free-mode
- [√] 4.2 在 `internal/api/openai/handler.go` 用 `feature_disable_models` 触发模型穿透；用 `feature_disable_billing` 判定 free mode（影响是否要求定价存在），验证 why.md#requirement-models-disable-model-passthrough

## 5. 迁移（停止并迁移）
- [√] 5.1 新增 SQL 迁移文件，在 `app_settings` 中将 `policy_*` 与 legacy models keys 映射到 `feature_disable_billing/feature_disable_models` 并清理旧 key，验证 why.md#requirement-migrate-legacy-policy-and-model-flags

## 6. 文档同步（SSOT）
- [√] 6.1 更新 `helloagents/wiki/api.md`、`helloagents/wiki/modules/realms.md`、`helloagents/wiki/data.md` 移除 policy 描述，改为 feature 语义（禁用计费=free mode；禁用模型=模型穿透）
- [√] 6.2 更新 `helloagents/CHANGELOG.md` 记录本次变更

## 7. 测试
- [√] 7.1 执行 `go test ./...` 并补齐必要的单测/集成测试（至少覆盖：模型穿透触发、free mode 触发、迁移映射）
