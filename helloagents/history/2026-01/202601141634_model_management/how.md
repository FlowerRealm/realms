# 技术设计: 模型管理（白名单 / 别名重定向 / 上游绑定 / 定价）

## 技术方案

### 核心技术
- Go `net/http`（现有服务）
- MySQL（内置 migrations）
- SSR 模板：`internal/admin` / `internal/web`

### 实现要点（KISS）
1. **模型目录（DB）是唯一来源（SSOT）**：`/v1/models`、白名单校验、用户模型页都以此为准。
2. **模型白名单在中转层强制**：请求在转发前校验，避免把错误传播给上游再“碰运气”。
3. **别名重定向只改写上游请求体**：下游看到/统计使用的仍是对外模型名，避免口径混乱。
4. **上游绑定通过“调度约束”实现**：复用现有 Scheduler 的冷却/RPM/粘性能力，避免在 handler 里复制调度逻辑。
5. **管理面只做必要字段**：不引入 new-api 的复杂模型同步/多租户/供应商体系。

## 架构设计

```mermaid
flowchart TD
  Client[客户端] -->|/v1/*| OpenAIHandler[internal/api/openai]
  OpenAIHandler -->|校验/重写 model| ModelCatalog[(managed_models)]
  OpenAIHandler -->|按模型约束选择| Scheduler[internal/scheduler]
  Scheduler -->|Selection| Executor[internal/upstream.Executor]
  Executor --> Upstream[上游 openai_compatible / codex_oauth]

  Root[管理员] --> AdminSSR[/admin/models / admin/pricing-models]
  AdminSSR --> Store[internal/store]
  User[用户] --> WebSSR[/models]
  WebSSR --> Store
```

## 架构决策 ADR

### ADR-004: 模型目录采用 DB 表作为 SSOT，并在数据面强制白名单
**上下文:** 中转服务必须可控、可审计；上游 models 能力不稳定且不应成为信任根。  
**决策:** 新增 `managed_models` 表；在 `/v1/*` handler 中强制校验；`GET /v1/models` 由本地输出。  
**理由:** 最简单且可运维；SSR 管理后台天然适配 DB 作为配置存储。  
**替代方案:** 配置文件维护模型清单 → 拒绝原因: 需要热更新/多环境一致性差/不便 UI 管理。  
**影响:** 初次上线需要先配置模型目录，否则强制模式会拒绝所有模型请求（需提供清晰提示/上线步骤）。

### ADR-005: 上游绑定通过 Scheduler 约束选择实现
**上下文:** 现有 Scheduler 已实现 promotion/affinity/RPM/cooldown/binding；复制逻辑会引入不一致与维护成本。  
**决策:** Scheduler 增加“按 upstream type / 指定 channel”约束的选择方法；handler 只负责计算约束并调用。  
**理由:** 复用已有可用性机制，保持行为一致；最小改动面。  
**替代方案:** handler 内手写“按 channel 选 credential” → 拒绝原因: 会绕开 scheduler 的状态管理与测试。  
**影响:** 需要处理 routeKey 绑定与模型约束冲突（不满足则清理绑定）。

## API 设计

### [GET] /v1/models
- **语义:** 返回本服务“可用模型目录”（启用模型集合），不再依赖上游透传。
- **响应（最小兼容 OpenAI）:**
```json
{
  "object": "list",
  "data": [
    { "id": "gpt-5.2", "object": "model", "created": 0, "owned_by": "openai_compatible" }
  ]
}
```

### 数据面强制策略（/v1/responses, /v1/chat/completions）
- **校验点:**
  - `model` 必须存在于 `managed_models` 且 `status=1`
  - 若模型绑定 `upstream_type=codex_oauth` 且请求为 `/v1/chat/completions` → 直接拒绝
- **改写点:**
  - 下游请求体 `model` 改写为 `upstream_model`
  - 计费/用量归因使用对外 `model`（而非 upstream_model）

### 管理面（SSR，root）
- `GET /admin/models`：列表 + 新增入口
- `POST /admin/models`：创建
- `GET /admin/models/{model_id}`：详情/编辑页
- `POST /admin/models/{model_id}`：更新（含启停、映射、绑定）
- `POST /admin/models/{model_id}/delete`：删除（硬删除）
- `GET /admin/pricing-models`：定价规则列表 + 新增入口
- `POST /admin/pricing-models`：创建
- `POST /admin/pricing-models/{id}`：更新（或拆分 /edit）
- `POST /admin/pricing-models/{id}/delete`：删除/禁用

## 数据模型

### 新增表: managed_models（建议命名）

```sql
CREATE TABLE IF NOT EXISTS `managed_models` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `public_id` VARCHAR(128) NOT NULL,
  `upstream_model` VARCHAR(128) NOT NULL,
  `owned_by` VARCHAR(64) NULL,
  `description` TEXT NULL,
  `upstream_type` VARCHAR(32) NOT NULL,
  `upstream_channel_id` BIGINT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_managed_models_public_id` (`public_id`),
  KEY `idx_managed_models_status` (`status`),
  KEY `idx_managed_models_upstream_type` (`upstream_type`),
  KEY `idx_managed_models_channel_id` (`upstream_channel_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：
- `public_id`: 对外暴露/校验的模型名（客户端使用）
- `upstream_model`: 发送给上游的模型名（alias → upstream）
- `upstream_type`: 约束调度范围（`openai_compatible` / `codex_oauth`）
- `upstream_channel_id`: 可选的“固定 channel”；为空则在 `upstream_type` 范围内由 Scheduler 选择
- `owned_by/description`: 用于展示（用户可见）

## 安全与性能
- **权限控制:** 管理面仅 root；用户侧只读展示，不暴露上游密钥与账号信息。
- **输入校验:** `public_id/upstream_model` 长度与字符集校验；`upstream_type` 必须为允许值；`upstream_channel_id` 若填写必须存在且类型匹配。
- **性能:** 每次请求多一次“按 public_id 查模型”的 DB 查询；当前中转路径已涉及配额/审计写入，增量可接受（可选：后续加短 TTL 缓存）。

## 测试与部署
- **迁移:** 新增 `internal/store/migrations/0007_managed_models.sql`（或下一个序号）创建表。
- **测试:**
  - handler：模型白名单拒绝、别名改写、绑定约束选择、routeKey 绑定冲突处理
  - scheduler：按 type/channel 约束选择的单测
- **发布步骤建议:**
  1) 部署新版本并完成迁移
  2) 在 `/admin/models` 先配置并启用模型目录
  3) 再开启“强制白名单”（如以配置开关形式提供）

