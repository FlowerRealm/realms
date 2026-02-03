# 技术设计: 缓存粘性路由最小增强

## 技术方案

### 核心技术
- Go `net/http`
- 现有 `middleware.BodyCache`（请求体缓存）
- 现有 `scheduler`（粘性绑定 + 冷却 + fail-score + RPM）
- 现有 `audit_events` 表与 `store.InsertAuditEvent`（仅关键事件）

### 实现要点

1. **RouteKey 提取口径确认（行为不变，补齐测试/文档）**
   - `openai` handler 已支持：优先读取 JSON body 顶层 `prompt_cache_key`；缺失时回退到 header（含常见大小写变体）。
   - routeKey 仅用于 hash（不落库/不打日志），并限制长度避免异常输入拖慢请求。
   - 本期补齐单元测试覆盖“body 优先 / header 兜底”的优先级与候选集合，避免后续回归。

2. **绑定命中与冷却联动（行为不变，补齐测试/文档）**
   - Scheduler 已具备：命中 binding、检查 cooling、命中后续期（TTL）；失败通过 `Report` 进入 cooling，使下一次 select 自动避开该凭证。
   - handler 维持整体尝试上限，每次 selection 仅尝试 1 次；本期补齐回归测试与文档口径。

3. **429 特例冷却（内存态）**
   - 在 `scheduler.Report` 中，当 `res.Retriable=true` 且 `res.StatusCode==429` 时，使用更长冷却窗口（2×基准冷却；默认 60s），其它可重试错误沿用基准冷却（默认 30s）。

4. **审计事件不泄漏约束（行为不变，补齐测试）**
   - 现有实现复用 `audit_events` 并记录 failover/上游错误（不记录请求体/明文凭据/原始 routeKey）。
   - 本期补齐单元测试：验证审计事件不会把上游错误 body 写入 `error_message`。

## 架构决策 ADR

### ADR-001: 本期不做跨实例的粘性/冷却持久化
**上下文:** `rc-balance` 使用 DO/D1 实现跨实例一致，而 Realms 目前是单体 Go 服务，路由状态默认内存。  
**决策:** 本期仅做最小增强（body routeKey + 冷却联动 + 429 特例），不引入 Redis，不新增 DB 表。  
**理由:** 变更小、风险可控、可快速验证收益；跨实例一致性属于更大改造（方案2/3）。  
**替代方案:** MySQL/Redis 持久化 → 拒绝原因: 需求边界与部署形态未确认，先做低风险收益点。  
**影响:** 多实例部署下粘性效果仍可能被 LB 稀释；但 routeKey 提取增强与 429 冷却仍有收益。

## 安全与性能

- **安全:**
  - routeKey 不落库、不输出日志；审计事件不记录敏感信息。
  - 维持现有请求体大小限制与上游响应体读取限制。
- **性能:**
  - routeKey 提取复用已解析的 JSON payload，无额外 body 读取。
  - 审计仅记录关键事件，避免每请求写库放大。

## 测试与部署

- **测试:**
  - `internal/api/openai`：新增单元测试覆盖 routeKey 提取优先级（body > header）与新 header 候选。
  - `internal/scheduler`：新增单元测试覆盖 429 冷却窗口长于默认冷却。
  - 回归：`go test ./...`
- **部署:**
  - 无 DB 迁移。
  - 建议先在低流量环境验证 429 比例、失败率、平均/尾延迟是否改善。
