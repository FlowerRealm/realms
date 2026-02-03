# 任务清单: Codex API 中转（Responses / Chat Completions / Models / /v1/*）

目录: `helloagents/plan/202601131834_codex_proxy/`

---

## 1. 项目骨架（Go 服务）
- [ ] 1.1 新建 Go 模块与基础目录结构（`cmd/server`、`internal/*`），并提供最小启动入口
- [ ] 1.2 实现配置加载（文件 + 环境变量覆盖），定义 `UpstreamConfig`（channels/baseUrls/apiKeys/priority/status/promotionUntil）

## 2. 认证与基础中间件
- [ ] 2.1 实现代理访问鉴权（`x-api-key` 或 `Authorization: Bearer`），与参考项目一致，覆盖 why.md#API-设计-认证
- [ ] 2.2 加入基础请求约束（请求体大小限制、超时），生成/透传 `request_id`

## 3. 调度与 failover（Channel → BaseURL → Key）
- [ ] 3.1 实现 ChannelScheduler：促销期 > Trace 亲和 > priority > fallback（失败率最低），覆盖 why.md#需求-多渠道优先级与自动切换
- [ ] 3.2 实现 TraceAffinity（TTL），从请求体 `user` 或 header `x-user-id` 提取（以实现阶段确认）
- [ ] 3.3 实现 URLManager：多 baseUrls 动态排序 + 冷却，覆盖 why.md#需求-多-baseurls-动态降级
- [ ] 3.4 实现 Key 冷却与指标熔断（滑窗失败率 + 最小样本保护），覆盖 why.md#需求-多-key-冷却与降权

## 4. 代理转发链路（/v1/*）
- [ ] 4.1 实现通用 `/v1/*` 代理：按选择的 baseUrl 构建目标 URL，复制请求头/体并注入上游 key（覆盖 `/v1/responses` 与 `/v1/chat/completions`）
- [ ] 4.2 实现错误分类（Normal + Fuzzy 模式）决定是否 failover；不可重试错误禁止重试
- [ ] 4.3 实现 SSE 透传：检测 `text/event-stream` 并按块转发 + flush；开始写回后禁止 failover

## 5. 内部接口与可观测性（最小集）
- [ ] 5.1 增加 `GET /healthz`（可选鉴权策略），输出基本运行状态与当前配置摘要（脱敏）
- [ ] 5.2 增加关键日志：选中渠道/端点/key（脱敏）、failover 原因、最终失败兜底原因

## 6. 安全检查
- [ ] 6.1 执行安全检查（按G9：输入验证、敏感信息处理、权限控制、SSRF 基础防护）

## 7. 文档更新（知识库同步）
- [ ] 7.1 更新 `helloagents/wiki/api.md`：补充对外代理接口与鉴权说明
- [ ] 7.2 更新 `helloagents/wiki/arch.md`：补充架构图与关键 ADR 索引
- [ ] 7.3 更新 `helloagents/wiki/data.md`：补充指标/状态数据模型（内存结构）
- [ ] 7.4 更新 `helloagents/CHANGELOG.md`：记录新增模块与关键能力

## 8. 测试
- [ ] 8.1 添加调度器单测：promotion/affinity/priority/fallback 的选择顺序
- [ ] 8.2 添加 failover 集成测试：key→url→channel 切换；覆盖 5xx/429/网络错误场景
- [ ] 8.3 添加 SSE 透传测试：event-stream 逐条转发且不阻塞（覆盖 `/v1/responses` 与 `/v1/chat/completions`）
