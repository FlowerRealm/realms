# 变更提案: 移除 policy_*，以功能禁用表达数据面语义

## 需求背景

当前系统同时存在两套开关概念：

- `feature_disable_*`：用于“隐藏入口 + 路由 404”（功能禁用）
- `policy_*`：用于“禁用=语义切换”（数据面策略）

这会带来配置入口分散、语义重复、以及“到底该关哪个”的理解成本。并且模型域目前拆成 `feature_disable_web_models` 与 `feature_disable_admin_models` 两个开关，无法表达“禁用就是全禁”的预期。

目标是删掉“数据面策略（policy_*）”这一独立维度，让“禁用开关”同时承载语义切换：

- 关闭计费域 ⇒ 数据面进入 **free mode**（不校验订阅/余额，但仍记录 `usage_events`）
- 关闭模型域 ⇒ 数据面进入 **模型穿透**（不要求模型启用/绑定，`model` 直接透传上游）

## 变更内容

1. 删除管理后台「系统设置」中的“数据面策略（Policy）”卡片与表单写入逻辑
2. 新增全局模型禁用开关 `feature_disable_models`，作为模型域唯一禁用入口，替代：
   - `feature_disable_web_models`
   - `feature_disable_admin_models`
3. 停止读取与生效：
   - `policy_free_mode`
   - `policy_model_passthrough`
   并将存量配置迁移到新的 feature 开关语义上（见“核心场景：迁移”）
4. 数据面语义由 feature 推导：
   - `feature_disable_billing=true` ⇒ free mode（等价于原 `policy_free_mode=true`）
   - `feature_disable_models=true` ⇒ 模型穿透（等价于原 `policy_model_passthrough=true`）

## 影响范围

- **模块:**
  - `internal/admin`（系统设置页展示/保存）
  - `internal/store`（开关键值、特性状态计算）
  - `internal/quota`（配额 provider 选择 free/normal）
  - `internal/api/openai`（模型白名单/绑定校验、模型穿透逻辑）
  - `internal/server`（路由 feature gate 绑定）
  - `internal/config`（配置文件默认值结构体/示例配置）
- **文件:**
  - Go：`internal/admin/*`、`internal/store/*`、`internal/quota/*`、`internal/api/openai/*`、`internal/server/*`、`internal/config/*`
  - 模板：`internal/admin/templates/*`、`internal/web/templates/*`
  - SQL：`internal/store/migrations/*`
  - 文档：`helloagents/wiki/*`、`helloagents/CHANGELOG.md`
- **API/页面:**
  - 管理后台：`/admin/settings`、`/admin/models*`
  - Web：`/models`
  - OpenAI 兼容：`/v1/responses`、`/v1/chat/completions`、`/v1/models`
- **数据:**
  - `app_settings` 中旧 key 的迁移与清理（不改表结构）

## 核心场景

### Requirement: billing-disable-free-mode
**模块:** `realms`（数据面配额/计费）

#### Scenario: billing-disabled
前置条件：
- 管理后台将 `feature_disable_billing=true`

预期：
- 相关计费/支付/订阅页面与接口保持 404（现有 feature gate 行为不变）
- 数据面请求不再返回 `订阅未激活/余额不足` 等计费相关拦截（进入 free mode）
- 仍记录 `usage_events`（用于统计与排障）

### Requirement: models-disable-model-passthrough
**模块:** `realms`（数据面模型解析与调度）

#### Scenario: models-disabled
前置条件：
- 管理后台将 `feature_disable_models=true`

预期：
- `/models` 直接 404（模型列表不再展示）
- `/admin/models*` 直接 404（模型管理不再可用）
- `/v1/responses` 与 `/v1/chat/completions` 进入模型穿透：
  - 不要求模型启用（跳过模型白名单）
  - 不要求渠道绑定存在（跳过 `channel_models` 白名单）
  - `model` 字段直接透传到上游（不做 alias rewrite）
  - 非 free mode 下仍要求模型定价存在（`managed_models` 中有记录）以维持计费口径

### Requirement: migrate-legacy-policy-and-model-flags
**模块:** `realms`（迁移）

#### Scenario: upgrade-migrates-app-settings
前置条件（任一成立）：
- `app_settings.policy_free_mode=true`
- `app_settings.policy_model_passthrough=true`
- `app_settings.feature_disable_web_models=true`
- `app_settings.feature_disable_admin_models=true`

预期：
- 自动写入：
  - `feature_disable_billing=true`（当 `policy_free_mode=true`）
  - `feature_disable_models=true`（当 `policy_model_passthrough=true` 或 legacy models 开关为 true）
- 自动删除旧 key（避免形成“僵尸配置”）

## 风险评估

- **风险:** 模型穿透会放宽输入约束，可能导致上游返回更多 4xx/5xx（例如 model 不存在）。
  - **缓解:** 在文档与 UI 提示中明确语义；并保持非 free mode 仍要求 `managed_models` 存在用于计费口径。
- **风险:** 迁移可能覆盖少量“手工写入 false”的异常配置。
  - **缓解:** 迁移仅在旧 key 明确为 `true` 时提升到新 key；并确保迁移幂等。
