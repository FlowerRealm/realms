# 任务清单: codex_responses_relay

目录: `helloagents/plan/202601131834_codex_responses_relay/`

---

## 1. 项目骨架与基础设施
- [ ] 1.1 初始化 Go 项目骨架（`cmd/` + `internal/` + 配置加载），验证 why.md#需求-codex-oauth-账号接入与刷新-场景-管理员导入-auth-cache
- [ ] 1.2 增加健康检查与版本信息（`GET /healthz` 等），验证 why.md#需求-日志脱敏与审计-场景-合规审计

## 2. OAuth 与凭据管理（控制面）
- [ ] 2.1 实现凭据导入接口（导入 `~/.codex/auth.json`），验证 why.md#需求-codex-oauth-账号接入与刷新-场景-管理员导入-auth-cache
- [ ] 2.2 实现 token 刷新服务（singleflight/分布式锁），验证 why.md#需求-codex-oauth-账号接入与刷新-场景-headless-onboarding云端
- [ ] 2.3 实现账号状态存储与加密（Postgres + KMS/主密钥），验证 why.md#需求-codex-oauth-账号接入与刷新-场景-管理员导入-auth-cache

## 3. Codex 上游执行器（数据面核心）
- [ ] 3.1 实现 Codex 请求构造（baseURL + headers 注入），验证 why.md#需求-responses-api-兼容含-sse-场景-sse-流式响应
- [ ] 3.2 实现 SSE 直通（上游→下游），支持断连/取消传播，验证 why.md#需求-responses-api-兼容含-sse-场景-sse-流式响应
- [ ] 3.3 实现非流式聚合（聚合到 completed），验证 why.md#需求-responses-api-兼容含-sse-场景-非流式响应

## 4. OpenAI 兼容 API
- [ ] 4.1 实现 `POST /v1/responses`（stream true/false），验证 why.md#需求-responses-api-兼容含-sse-场景-sse-流式响应
- [ ] 4.2 实现 `GET /v1/models`（含 alias/过滤），验证 why.md#需求-models-api-场景-工具探测模型列表
- [ ] 4.3 评估并实现 Responses 扩展端点（retrieve/cancel/compact/input_items），验证 why.md#需求-responses-api-兼容含-sse-场景-非流式响应

## 5. 多账号轮询与熔断
- [ ] 5.1 实现账号选择策略（round-robin + 可用性过滤），验证 why.md#需求-多账号轮询与故障隔离-场景-账号被限流配额不足
- [ ] 5.2 实现错误分类与冷却退避（429/403/5xx/网络），验证 why.md#需求-多账号轮询与故障隔离-场景-账号被限流配额不足
- [ ] 5.3 实现恢复探测与告警指标，验证 why.md#需求-多账号轮询与故障隔离-场景-账号被限流配额不足

## 6. 限流与配额保护
- [ ] 6.1 实现 per-client（API key/租户）限流，验证 why.md#需求-限流与配额保护-场景-突发流量保护
- [ ] 6.2 实现 per-upstream-account 限流，验证 why.md#需求-限流与配额保护-场景-突发流量保护

## 7. 安全与可观测
- [ ] 7.1 实现日志脱敏中间件（headers/body 规则），验证 why.md#需求-日志脱敏与审计-场景-合规审计
- [ ] 7.2 增加审计事件与结构化日志字段（request_id/tenant/model），验证 why.md#需求-日志脱敏与审计-场景-合规审计
- [ ] 7.3 增加 metrics/tracing（OTel），验证 why.md#需求-日志脱敏与审计-场景-合规审计

## 8. 部署与文档
- [ ] 8.1 容器化与云部署清单（K8s/Helm 或等价），验证 why.md#需求-日志脱敏与审计-场景-合规审计
- [ ] 8.2 更新知识库：架构/API/运维说明，验证 why.md#需求-日志脱敏与审计-场景-合规审计

## 9. 安全检查
- [ ] 9.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

