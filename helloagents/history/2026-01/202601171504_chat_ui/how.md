# 技术设计: 用户对话功能（按分组固定渠道）

## 技术方案

### 核心技术
- **后端:** Go（`net/http`）+ SSR（`html/template`）
- **数据面:** 复用现有 `POST /v1/responses`（支持 SSE）
- **鉴权:** 复用现有数据面 Token（`Authorization: Bearer rlm_...`）
- **前端:** 最小 JS（`fetch` + `EventSource`/`ReadableStream` 解析 SSE），对话记录仅保存 `localStorage`
- **存储:** MySQL 新增一张轻量配置表（不存对话内容）

### 实现要点

1. **对话页只做“客户端 UI”**：不在服务端保存对话消息，避免扩大敏感数据面；对话历史由浏览器 `localStorage` 保存。
2. **Token 自动化**：对话页通过会话态接口获取/轮换一个专用 `chat` token，并在浏览器保存，用于调用数据面 `/v1/responses`（沿用计费/配额）。
3. **对话分组路由约束**：对话页请求携带对话标识（建议 `X-Realms-Chat: 1`），服务端据此启用“固定渠道”约束：
   - 根据用户所属组计算对话分组（优先非 `default`；无匹配则回退 `default`）
   - 查表得到 `upstream_channel_id`
   - 将调度约束设为 `RequireChannelID = <绑定渠道>`，从而禁止跨 channel failover
4. **用户可选模型，但必须在对话渠道可用**：对话页模型下拉按“对话渠道绑定模型”过滤；服务端也做兜底校验并返回明确错误。

## 架构设计

```mermaid
flowchart TD
  Browser[浏览器 /chat] -->|Cookie Session| Web[Web SSR: /chat]
  Browser -->|POST /api/chat/token (CSRF)| Web
  Browser -->|GET /api/chat/models| Web

  Browser -->|POST /v1/responses<br/>Authorization: Bearer rlm_...<br/>X-Realms-Chat: 1| OpenAI[OpenAI Handler]
  OpenAI -->|固定 Channel 约束| Scheduler[Scheduler]
  Scheduler --> Executor[Upstream Executor]
  Executor --> Upstream[(OpenAI compatible / Codex OAuth)]

  OpenAI --> Store[(MySQL)]
  Web --> Store
  Admin[Admin SSR] --> Store
```

## 架构决策 ADR

### ADR-010: 对话分组路由采用 `group_name → upstream_channel_id` 映射（不引入新“租户/分组体系”）
**上下文:** 系统已有 `user_groups` / `upstream_channels.groups` / `channel_groups`，且调度已按分组过滤渠道。  
**决策:** 新增 `chat_group_routes` 表，使用既有 `group_name` 作为键，映射到固定 `upstream_channel_id`。  
**理由:** 最小新增概念，复用现有分组治理与管理后台能力，落地快且可维护。  
**替代方案:** 新建 `chat_groups` + 用户/渠道绑定体系 → 拒绝原因: 概念重复、成本高、容易与现有分组冲突。  
**影响:** 需要定义“用户属于多个组时如何选择对话分组”的确定性规则（见下文）。

### ADR-011: 通过请求头标识对话请求（仅对对话页生效）
**上下文:** 数据面接口也可被 CLI/SDK 调用，不应强制所有请求都走固定渠道。  
**决策:** 对话页请求附带 `X-Realms-Chat: 1`，服务端仅在该标识存在时启用“固定渠道”约束。  
**替代方案:** 仅凭 token 名称判断（chat token 强制固定）→ 拒绝原因: 需在数据面鉴权额外查询 token 名称，且会影响 token 的通用性。  
**影响:** 对话页需要显式设置该请求头；非对话页调用不受影响。

## 对话分组选择规则（用户多组）

目标：简单、确定、可解释，并以 `default` 作为兜底。

规则：
1. 获取用户组集合（`principal.Groups`，必含 `default`）。
2. 先按“非 default 组”匹配 `chat_group_routes`（按字典序或按固定顺序遍历即可，保证确定性）。
3. 若无匹配，则回退 `default` 的路由配置。
4. 若连 `default` 也未配置，则返回明确错误提示“未配置对话渠道，请联系管理员”。

> 说明：本规则避免“default 抢占”导致高级分组路由失效。

## API 设计

### 用户侧（Web）

#### [GET] /chat
- **描述:** 用户对话页面（SSR）。
- **行为:** 页面提供模型选择、对话框、历史管理；对话内容仅本地保存。

#### [POST] /api/chat/token
- **描述:** 下发或轮换 `chat` token（会话鉴权 + CSRF）。
- **请求:** `application/json` 或 `application/x-www-form-urlencoded`（实现期二选一，建议 JSON）  
  可选参数：`rotate=true`（明确轮换）。
- **响应:** `{"token":"rlm_...","hint":"...optional"}`（仅本次返回明文）

#### [GET] /api/chat/models
- **描述:** 返回“当前用户对话渠道”可用的模型列表（按 `managed_models` + `channel_models` 过滤）。
- **响应:** `{"models":[{"id":"gpt-4.1-mini","owned_by":"realms"}]}`（结构可复用 `/v1/models` 的 item）

### 管理侧（Admin，root）

#### [GET] /admin/chat-routes
- **描述:** 对话分组路由管理页（SSR）。
- **展示:** 分组列表、当前绑定渠道、渠道健康/状态、保存入口。

#### [POST] /admin/chat-routes
- **描述:** 新增/更新 `group_name → upstream_channel_id` 绑定（CSRF）。
- **校验:**
  - `group_name` 合法且存在于 `channel_groups`
  - `upstream_channel_id` 存在且启用
  - `upstream_channels.groups` 必须包含该 `group_name`

#### [POST] /admin/chat-routes/{group_name}/delete
- **描述:** 删除某分组的对话路由绑定（CSRF）。

### 数据面（OpenAI 兼容）

#### [POST] /v1/responses
- **变更:** 若请求头存在 `X-Realms-Chat: 1`，则启用固定渠道约束：只允许调度到 `chat_group_routes` 绑定的渠道内。
- **其他行为:** 维持现有 SSE/计费/用量统计逻辑。

## 数据模型

新增表（建议）：

```sql
CREATE TABLE IF NOT EXISTS `chat_group_routes` (
  `group_name` VARCHAR(64) NOT NULL,
  `upstream_channel_id` BIGINT NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`group_name`),
  KEY `idx_chat_group_routes_channel_id` (`upstream_channel_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：
- 主键使用 `group_name`，避免额外引入 `group_id` 概念；与 `user_groups.group_name`、`upstream_channels.groups`、`channel_groups.name` 保持一致。

## 安全与性能

- **安全**
  - `/api/chat/token` 必须使用会话鉴权 + CSRF，防止第三方站点触发 token 轮换
  - 对话页不渲染富文本，所有用户输入以纯文本展示，避免 XSS
  - 对话请求头标识仅用于“收敛渠道”，不会扩大权限面
- **性能**
  - 对话路由查询仅对带对话标识的请求触发；必要时可加短 TTL 内存缓存（本期不强制）
  - `/api/chat/models` 可按用户对话渠道过滤，减少“选了不可用模型再失败”的无效请求

## 测试与部署

- **测试**
  - Store 层：对话路由选择规则（多组/default 回退）
  - OpenAI handler：对话标识请求必须命中 `RequireChannelID`，且模型未绑定时返回明确错误
  - Web：token 轮换接口权限/CSRF 验证
- **部署**
  - 追加 MySQL 迁移文件（服务启动自动迁移）
  - 不引入新外部依赖，部署方式保持不变

