# 变更提案: 模型管理（白名单 / 别名重定向 / 上游绑定 / 定价）

## 需求背景

当前系统作为 Codex/OpenAI API 中转，已经支持：
- 数据面：`/v1/responses`、`/v1/chat/completions`、`/v1/models`
- 管理面：上游 Channel/Endpoint/Credential/Account 配置
- 计费/限额：`usage_events` + `pricing_models`（按 pattern 匹配）

但“模型”仍主要依赖上游透传与上游自身的可用性/权限，不满足中转服务在企业内落地的核心诉求：
1. **可控性**：必须能明确“哪些模型可用”，并拒绝名单外模型，避免误用/越权/意外成本。
2. **一致性**：同一个“对外模型名”在不同上游可能需要映射（alias → upstream），否则客户端侧无法稳定使用。
3. **可运维性**：管理员需要在管理后台维护模型信息与定价规则；普通用户仅查看可用模型与说明信息。
4. **路由能力**：同一套北向 API 下，不同模型可能需要绑定不同上游（`openai_compatible` / `codex_oauth` 或指定 channel）。

## 产品分析

### 目标用户与场景
- **管理员（root）**：维护模型目录、模型别名/映射、模型与上游绑定、模型定价规则；对外提供稳定的模型清单。
- **普通用户**：查看可用模型列表与说明信息；在调用时只能使用允许模型。
- **平台/运维**：希望“模型管控”成为中转层的能力，而不是寄希望于上游或客户端自律。

### 价值主张与成功指标
- **价值主张**：把“可用模型集合”从上游不确定性中剥离，收敛为本系统可审计、可配置、可强制的规则。
- **成功指标**
  - 数据面：`/v1/models` 始终可用（不依赖上游是否提供 models），返回受控模型列表。
  - 强制策略：`/v1/responses`、`/v1/chat/completions` 对名单外模型请求统一拒绝。
  - 路由：同一用户的不同模型请求能按绑定命中目标上游，并具备最小 failover 行为。
  - 可运维：管理员可在 `/admin` 管理模型与定价；普通用户在 `/models` 查看模型信息。

### 人文关怀
- **减少误伤**：对被拒绝的模型给出清晰可行动的错误提示（例如“请联系管理员启用该模型”）。
- **隐私与安全**：用户侧只展示“模型信息”，不暴露上游 credential/account 等敏感细节。

## 变更内容
1. 新增“模型目录”（DB 表）作为可用模型的 SSOT：支持启用/禁用、描述、别名映射与上游绑定。
2. 数据面 `GET /v1/models` 改为从模型目录输出（OpenAI models list 风格）。
3. 数据面 `POST /v1/responses` 与 `POST /v1/chat/completions`：
   - 请求解析后做模型白名单校验（拒绝名单外）
   - 对命中模型执行别名重定向（对上游请求体改写为 upstream model）
   - 按模型绑定选择上游（固定 channel / 限定 upstream type）
4. 管理面（SSR）新增：
   - `/admin/models`：模型目录管理（增删改、启停、映射、绑定）
   - `/admin/pricing-models`：定价表 `pricing_models` 管理（增删改/启停；pattern + priority）
5. Web 控制台（SSR）`/models`：从模型目录展示“可用模型信息”（普通用户只读）。

## 影响范围
- **模块:**
  - `internal/store`（新增模型目录表与 CRUD；扩展定价表 CRUD）
  - `internal/api/openai`（模型校验/重写/路由约束；`/v1/models` 输出改造）
  - `internal/scheduler`（支持“按模型约束”选择上游）
  - `internal/admin`（新增 models/pricing 管理页面）
  - `internal/web`（用户模型页改为读 DB）
- **API:**
  - `GET /v1/models` 行为变更（由上游透传 → 本地目录输出）
  - 管理面新增：`/admin/models`、`/admin/pricing-models`
- **数据:**
  - 新增模型目录表（例如 `managed_models`）
  - `pricing_models` 增强管理能力（不变更计费口径）

## 核心场景

### 需求: 模型目录管理（管理员）
**模块:** admin/store
管理员可维护“可用模型集合”，作为系统对外暴露与强制校验的来源。

#### 场景: root 在管理后台维护模型
管理员登录 `/admin/models`：
- 可新增模型（对外模型名、上游模型名、描述、状态、绑定上游）
- 可编辑/禁用/删除模型
- 禁用后模型从 `/v1/models` 与 `/models` 消失，并拒绝该模型的调用

### 需求: 模型白名单强制（数据面）
**模块:** api/openai
对数据面请求做强制白名单：名单外模型统一拒绝。

#### 场景: 请求使用未允许模型
`POST /v1/responses` 或 `POST /v1/chat/completions` 请求体 `model=xxx`：
- 若 `xxx` 不在模型目录或处于禁用状态 → 返回错误（提示联系管理员启用/配置）

### 需求: 模型别名重定向（alias → upstream）
**模块:** api/openai
对命中模型进行 model 字段重写，将对外模型名映射到上游真实模型名。

#### 场景: 客户端使用别名
客户端请求 `model="gpt-4.1-mini"`，模型目录配置 upstream 为 `"gpt-4.1-mini-2025-xx"`：
- 上游请求体中的 `model` 被改写为 upstream model
- 对用户侧日志/用量归因以对外模型名为主（避免成本与显示混乱）

### 需求: 上游绑定模型（按模型路由）
**模块:** scheduler/api/openai
模型可绑定到：
- 指定 upstream type（`openai_compatible` / `codex_oauth`）
- 或指定 channel（固定命中某个 channel）

#### 场景: 不同模型走不同上游
当模型绑定为 `codex_oauth`：
- 仅允许在 `POST /v1/responses` 使用；`/v1/chat/completions` 请求应明确拒绝（避免“看似成功但实际不可用”）

当模型绑定为 `openai_compatible`（或固定到某个 openai channel）：
- 走现有 key 轮询与 failover 逻辑

### 需求: 用户可见模型信息（只读）
**模块:** web
普通用户可在 `/models` 查看可用模型与说明信息，但无权修改。

#### 场景: 用户查看模型列表
用户访问 `/models`：
- 展示启用模型的 `id` + `owned_by/渠道类型` + `描述`
- 不展示任何敏感信息（credential/account）

### 需求: 定价表管理（管理员）
**模块:** admin/store/quota
管理员可维护 `pricing_models` 以支撑成本估算与订阅限额。

#### 场景: root 维护定价规则
管理员在 `/admin/pricing-models`：
- 可新增/编辑/禁用定价规则（pattern + priority + input/output 单价）
- 变更立即影响后续用量成本估算（不回溯历史记录）

## 风险评估
- **风险: 误配导致全量请求被拒绝**
  - **缓解:** 提供清晰错误提示；管理后台给出“当前启用模型数/定价规则数”的可观测性；（可选）提供“禁用强制”的配置开关用于紧急恢复。
- **风险: 路由粘性与模型绑定冲突**
  - **缓解:** 当 routeKey 命中绑定但不满足模型约束时，清理绑定并重新选择。
- **风险: 兼容性变化**
  - **缓解:** 在上线前明确迁移步骤（先配置模型目录，再开启强制策略）；对 `GET /v1/models` 行为变更在 Changelog 中标注。

