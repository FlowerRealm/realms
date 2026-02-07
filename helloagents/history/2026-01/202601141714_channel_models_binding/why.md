# 变更提案: 渠道绑定模型（替代“模型绑定渠道”）

## 需求背景

上一版实现将“上游绑定”放在模型维度（`managed_models.upstream_type/upstream_channel_id`），但真实运维习惯更符合：

- **渠道（channel）配置自己支持哪些模型**（以及对应的 upstream model 映射）
- 同一 public model 可被多个渠道支持，用于 **failover/容灾**

因此需要把“绑定关系”的 SSOT 从“模型→渠道”调整为“渠道→模型”。

## 变更内容
1. 引入渠道-模型绑定表（例如 `channel_models`），支持：
   - `channel_id` 绑定 `public_id`
   - 每个绑定可配置 `upstream_model`（alias → upstream）
   - 绑定可启用/禁用
2. `managed_models` 回归为“全局模型目录（元信息）”：
   - `public_id`、`owned_by`、`description`、`status`
3. 数据面请求路由：
   - 先用 `managed_models` 做“模型白名单（全局启用）”
   - 再用 `channel_models` 得到“允许的渠道集合”
   - 调度只在该集合内选择渠道；选中渠道后使用该渠道的 `upstream_model` 改写请求体
4. 管理后台：
   - `/admin/models` 管理全局模型目录（元信息）
   - `/admin/channels/{id}/models` 管理该渠道绑定的模型集合与 upstream_model 映射

## 影响范围
- **模块:** store / scheduler / api(openai) / admin / web
- **API:** `/v1/models` 输出口径仍为“受控模型列表”，但其来源改为“至少存在一个可用渠道绑定”的模型集合
- **数据:** 新增 `channel_models` 表；`managed_models` 列定义调整（上游字段不再作为 SSOT）

## 核心场景

### 需求: 渠道绑定模型（管理员）
**模块:** admin/store
管理员为某个渠道配置可用模型集合，并为每个模型配置 upstream_model（可选）。

#### 场景: 一个模型由多个渠道提供
- `public_id=gpt-5.2` 可同时绑定到多个 `openai_compatible` 渠道
- 请求到来时，只在这些渠道内做 failover

### 需求: 强制白名单 + alias 重写（数据面）
**模块:** api/openai
- `managed_models` 未启用 → 拒绝
- `channel_models` 无可用绑定 → 拒绝
- 选中渠道后改写 payload `model=upstream_model`

## 风险评估
- **风险:** 迁移后模型“未绑定渠道”会导致请求被拒绝
  - **缓解:** 管理后台在渠道页提供绑定入口；`/v1/models` 只展示“至少有一个可用绑定”的模型，降低误用。

