# 任务清单: Codex OAuth 会话粘性绑定与 RPM 负载均衡

> 说明：本方案包已合并到 `helloagents/history/2026-01/202601131914_codex/`，此处仅保留归档副本（未执行）。

目录: `helloagents/plan/202601132055_codex_oauth_sticky_balance/`

---

## 1. routeKey 提取
- [ ] 1.1 在 `internal/router/route_key.go` 实现 routeKey 提取与规范化（优先级见 why.md#需求-routekey-提取与会话绑定）
- [ ] 1.2 添加单测 `internal/router/route_key_test.go` 覆盖 4 种来源与优先级

## 2. 会话绑定存储（TTL=30min，内存）
- [ ] 2.1 在 `internal/router/session_binding_store.go` 实现 `Get/Set/Touch/Delete` + 过期清理循环（key 使用 routeKeyHash）
- [ ] 2.2 添加单测 `internal/router/session_binding_store_test.go` 覆盖：写入、续期、过期回收

## 3. RPM 统计（rolling window）
- [ ] 3.1 在 `internal/router/rpm_counter.go` 实现 rolling RPM（窗口 60s），提供 `Inc(accountId)` 与 `GetRPM(accountId)`
- [ ] 3.2 添加单测 `internal/router/rpm_counter_test.go` 覆盖：窗口滚动、tie-break 可预期

## 4. Codex OAuth 账号选择 + 重试→重绑
- [ ] 4.1 在 `internal/router/codex_oauth_sticky_router.go` 实现：命中绑定优先、失败先重试 3 次（所有错误）、再按 RPM 最低重绑并继续（带总尝试上限与 excluded set）
- [ ] 4.2 在相关 handler（`/v1/responses` 与 `/v1/chat/completions`）接入“流式边界”：未写回前允许重试/重绑；写回后禁止 failover（why.md#需求-重试→重绑）
- [ ] 4.3 将 **每次上游尝试（含重试）** 计入 RPM，保证负载口径与真实压力一致

## 5. 可用性判定（最小）
- [ ] 5.1 定义账号可用性过滤：disabled/缺失/不支持模型（如有模型过滤机制）
- [ ] 5.2 为失败重绑维护 `excludedAccountIds`，避免同请求内回环

## 6. 安全检查
- [ ] 6.1 执行安全检查（按G9：敏感信息处理、重试上限、防止死循环、SSE 写回后禁止 failover）

## 7. 测试
- [ ] 7.1 添加路由单测：粘性命中、TTL 续期、失败重试满 3 次才重绑、RPM 选择最小值
- [ ] 7.2 添加 SSE 集成测试：首字节前失败可重试；首字节后失败不切换

## 8. 文档更新
- [ ] 8.1 更新 `helloagents/wiki/modules/codex.md`：补充本方案包入口与关键行为摘要
