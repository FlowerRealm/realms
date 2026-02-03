# 技术设计: 移除 group 概念（单租户化）

## 技术方案

### 核心技术
- Go（`net/http`）
- MySQL（迁移文件驱动）

### 实现要点
- **Schema 收敛**
  - 删除 `groups` 表
  - 删除所有表上的 `group_id` 字段与索引（`users`/`user_tokens`/`upstream_channels`/`audit_events`/`pricing_models`/`usage_events`/`user_subscriptions`）
  - `user_subscriptions` 以 `user_id` 唯一约束
  - `pricing_models` 仅保留全局定价字段（不再区分 scope/group）
- **鉴权与主体信息**
  - `auth.Principal` 移除 `GroupID`
  - Token 鉴权通过 `user_tokens JOIN users` 获取 `user_id/token_id/role`
  - Session 鉴权通过 session → user 获取 `user_id/role`
- **调度器去 group 化**
  - `scheduler.Select(...)` 不再接受 `group_id`，绑定/亲和 key 仅以 `user_id` + `route_key_hash` 维度存储
  - `ListUpstreamChannelsByGroup` 改为 `ListUpstreamChannels`（全局），保持 promotion/priority/rpm/cooldown 逻辑不变
- **配额与计费**
  - `usage_events` 汇总与订阅限额按 `user_id` 维度计算
  - 成本估算仅使用全局 `pricing_models`（pattern 优先级匹配）
- **管理后台与 Web 控制台**
  - 移除 `/admin/groups` 页面与所有 group 过滤/输入框
  - `/admin/*` 仅允许 `root` 访问（不再存在 group_admin 语义）
  - Web 控制台不再展示分组信息，管理员入口仅对 `root` 可见
- **用量 API（参考 new-api）**
  - 新增 `GET /api/usage/windows`：返回 5h/7d/30d 窗口汇总（口径与 `/subscription` 一致）
  - 新增 `GET /api/usage/events`：分页返回最近用量事件（仅当前用户）
  - 接口风格参考 `/home/flowerrealm/new-api` 的 usage 路由（`router/api-router.go`）与 token usage 返回结构，但不引入其复杂度（KISS）。

## 架构决策 ADR

### ADR-006: 移除 group/租户维度
**上下文:** 本项目作为中转服务，上游由管理员统一控制；多租户 group 带来额外隔离语义与维护成本。  
**决策:** 系统收敛为单租户：删除 group 相关数据结构与接口；所有配置全局化。  
**理由:** 降低复杂度，减少越权/计费混乱的可能性，符合“上游由管理员控制”的产品定位。  
**替代方案:** 保留 group 但固定 `default` → 拒绝原因: 代码与数据模型仍残留 group 概念，无法满足“删掉 group 概念”的目标。  
**影响:** 需要重写迁移并大范围重构；由于 DB 可全删，迁移成本可控。  

## API设计

### [GET] /api/usage/windows
- **鉴权:** Cookie 会话（SessionAuth）；可选增加 TokenAuth 同一套返回结构
- **响应:** 返回最近 5h/7d/30d 的 `committed/reserved/limit/remaining` 汇总（与 `/subscription` 页面一致）

### [GET] /api/usage/events
- **鉴权:** Cookie 会话（SessionAuth）；可选增加 TokenAuth
- **查询参数（建议）:**
  - `limit`：默认 50，最大 200
  - `before_id`：用于向前翻页
- **响应:** 返回 `usage_events`（time/request_id/state/model/input_tokens/output_tokens/usd_micros）列表，仅包含当前用户数据

## 数据模型

```sql
-- 仅示意关键变化（以 migrations 为准）
-- - 删除 groups 表
-- - users/user_tokens/upstream_channels/audit_events/usage_events/user_subscriptions 移除 group_id
-- - pricing_models 移除 scope/group_id，仅保留全局定价
```

## 安全与性能

- **安全:**
  - 新增 usage API 必须严格按 `principal.user_id` 过滤，禁止传入任意 user_id 查询
  - 保持 SSRF 校验逻辑不变（Endpoint base_url 校验仍在 admin 侧执行）
  - 不在日志/审计中记录任何明文 Token/Key
- **性能:**
  - `usage_events` 汇总查询按 `user_id + time + state` 索引设计，避免全表扫描
  - `usage/events` 分页使用 `id` 或 `time` 作为游标

## 测试与部署

- **测试:** `go test ./...`（含 scheduler 与 openai handler 相关单测）
- **部署:** 重建数据库并执行迁移；由于 schema 破坏性变更，不提供旧数据迁移路径（已确认可全删）

