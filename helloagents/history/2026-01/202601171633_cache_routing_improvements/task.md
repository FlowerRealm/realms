# 任务清单: 缓存粘性路由最小增强

目录: `helloagents/plan/202601171633_cache_routing_improvements/`

---

## 1. RouteKey 提取增强（body 优先）
- [√] 1.1 在 `internal/api/openai/handler.go` 中实现 routeKey 提取：优先 `payload.prompt_cache_key`，header 兜底（新增 `x-prompt-cache-key`/`x-rc-route-key` 等），验证 why.md#需求-routekey-提取body-优先-场景-body-含-prompt_cache_key无-header
  > 备注: 代码中已存在该逻辑（body 优先 + header 兜底）。
- [√] 1.2 在 `internal/api/openai/handler_test.go` 中补充单元测试覆盖 body/header 优先级与候选头部集合，验证 why.md#需求-routekey-提取body-优先-场景-header-兼容更广候选

## 2. 绑定命中与冷却联动（避免无意义重复重试）
- [√] 2.1 在 `internal/api/openai/handler.go` 中移除/简化“命中绑定固定重试”分支，统一走 `scheduler.SelectWithConstraints`，每 selection 仅尝试 1 次并保留整体尝试上限，验证 why.md#需求-绑定命中与冷却联动-场景-命中绑定但凭证已冷却
  > 备注: handler 已统一走 `scheduler.SelectWithConstraints`（无固定重试分支）。
- [√] 2.2 执行回归：确保既有 failover 行为与 group/chat 约束不被破坏（更新必要测试），验证 why.md#需求-绑定命中与冷却联动

## 3. 429 冷却更长（内存态）
- [√] 3.1 在 `internal/scheduler/scheduler.go` 中对 `StatusCode==429` 的可重试失败应用更长冷却窗口（例如 60s），验证 why.md#需求-429-冷却更长-场景-上游-429-后短期内不再选择同一凭证
- [√] 3.2 在 `internal/scheduler/scheduler_test.go` 中补充单元测试验证 429 冷却窗口，验证 why.md#需求-429-冷却更长

## 4. 基础审计事件（路由关键事件）
- [√] 4.1 在 `internal/api/openai/handler.go` 增加可选 `AuditSink` 并在 `internal/server/app.go` 注入（复用 `store.InsertAuditEvent`），验证 why.md#需求-基础审计事件路由关键事件
  > 备注: 代码中已存在审计写入（failover/upstream_error），本期补齐测试防回归。
- [√] 4.2 在 `internal/api/openai/handler_test.go` 增加 fake audit sink，验证 failover/最终错误路径会触发审计写入且不包含敏感信息，验证 why.md#需求-基础审计事件路由关键事件-场景-failover上游错误可追溯

## 5. 安全检查
- [√] 5.1 安全检查：确认不记录请求体/明文凭据；审计字段长度限制；routeKey 不落库不打日志（按G9）
  > 备注: 新增测试覆盖“审计不记录上游错误 body”的约束。

## 6. 文档更新
- [√] 6.1 更新 `helloagents/wiki/modules/realms.md`：补充 routeKey 提取顺序（body 优先）、header 兜底集合、429 冷却口径与审计事件说明

## 7. 测试
- [√] 7.1 运行 `go test ./...` 并修复本次变更引入的问题
