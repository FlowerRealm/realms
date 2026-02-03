# 变更提案: 模型定价（input/output/cache）与移除 legacy 上游字段

## 需求背景

当前 `managed_models` 已作为数据面模型白名单与展示元信息的 SSOT，但历史遗留的 `upstream_type/upstream_channel_id` 字段会引入调度歧义（模型维度绑定 vs 渠道维度绑定），且难以与现行 `channel_models` 的“按渠道绑定模型”模式保持一致。

同时，计费侧需要支持 prompt cache 命中场景，因此模型需要具备可配置的 **输入/输出/缓存** 三类单价，以便把 token 用量换算为 `usd_micros`。

## 变更内容

1. 移除 `managed_models` 的 legacy 字段：`upstream_type`、`upstream_channel_id`。
2. 为 `managed_models` 增加按模型定价字段：`input_usd_micros_per_1m`、`output_usd_micros_per_1m`、`cache_usd_micros_per_1m`。
3. 管理后台模型页支持配置上述三类价格。
4. 数据面调度统一以 `channel_models` 绑定为准；计费按缓存 token 使用 cache 单价结算。

## 影响范围

- **模块:**
  - `internal/store`（数据结构、SQL、迁移）
  - `internal/admin`（模型管理表单）
  - `internal/api/openai`（模型路由/调度逻辑）
  - `internal/quota`（成本计算）
- **文件:**
  - `internal/store/migrations/0011_managed_models_pricing.sql`
  - `internal/store/models.go`
  - `internal/store/managed_models.go`
  - `internal/admin/models.go`
  - `internal/admin/templates/models.html`
  - `internal/api/openai/handler.go`
  - `internal/quota/quota.go`
  - `helloagents/wiki/data.md`
  - `helloagents/CHANGELOG.md`
- **数据:**
  - `managed_models` 表结构变更（删除两列 + 新增三列）

## 核心场景

### 需求: 模型定价
**模块:** 管理后台 / 计费

#### 场景: 管理员配置模型价格
- 管理员可在 `/admin/models` 创建/编辑模型时填写 input/output/cache 单价（`usd_micros/1M tokens`）。
- 模型价格用于后续用量结算的成本换算。

### 需求: 调度一致性
**模块:** 数据面

#### 场景: 请求转发与模型绑定
- `/v1/responses`、`/v1/chat/completions` 的可用渠道集合仅由 `channel_models` 决定（不再读取 `managed_models` 的 legacy 上游字段）。
- 无可用绑定时拒绝请求；若存在绑定但 chat 无可用 `openai_compatible`，提示仅支持 `/v1/responses`。

## 风险评估

- **风险:** 历史部署可能仍依赖 `managed_models.upstream_type/upstream_channel_id` 的回退逻辑。
- **缓解:** 明确以 `channel_models` 作为唯一调度依据；管理后台引导用户在渠道页配置绑定；数据库迁移提供默认安全单价避免计费为 0。
