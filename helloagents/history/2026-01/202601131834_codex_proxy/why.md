# 变更提案: Codex API 中转（Responses / Chat Completions / Models / /v1/*）

## 需求背景
当前需要一个“Codex API 中转/网关”，对客户端提供 OpenAI 风格 API（至少包含 `/v1/responses`、`/v1/chat/completions`、`/v1/models`，以及可扩展的其他 `/v1/*` 接口），并在上游侧支持多渠道、多端点、多密钥的自动切换与优先级调用，以提升可用性并降低单点风险。

关键约束与偏好（来自需求确认）：
1. 对外暴露：`responses` + `chat/completions` + `models` + 其他接口（优先做成通用 `/v1/*` 转发）。
2. 上游：支持多 `baseUrl/baseUrls`；“OpenAI 官方 Codex 渠道”作为内置可用选项。
3. 多渠道调度：参考 `claude-proxy` 的策略（优先级 + 自动 failover）。
4. 客户端鉴权：使用 `x-api-key` 或 `Authorization: Bearer ...`（与参考项目一致）。
5. 实现方式：重新搭建（不直接复用现有大型工程）。

## 产品分析

### 目标用户与场景
- **用户群体:** 需要稳定访问 Codex/OpenAI API 的开发者/服务端调用方
- **使用场景:** 内网/受限网络环境；多出口/多线路；多账号/多 key 容灾；需要统一鉴权与审计
- **核心痛点:** 单一上游失败导致整体不可用；某些 key 配额/限流导致请求大量失败；上游端点抖动导致短期雪崩

### 价值主张与成功指标
- **价值主张:** 在不改变客户端调用方式的前提下，提供高可用、可配置、可观测的中转层
- **成功指标:**
  - 业务请求在单渠道/单端点/单 key 失效时仍能自动切换成功（可通过测试复现）
  - SSE 流式响应可稳定透传（不丢事件、不缓存导致卡顿）
  - 错误分类可控：不可重试错误不做无意义 failover；Fuzzy 模式下尽量“先跑通”

### 人文关怀
- 默认最小化日志中敏感信息（不记录完整请求体/密钥明文），避免无意泄漏。

## 变更内容
1. 新建一个轻量 Codex Proxy 服务：统一入口、统一鉴权、统一转发链路。
2. 引入“多渠道 + 多端点 + 多密钥”的三层 failover，并实现优先级调用。
3. 实现可配置的错误分类策略（含 Fuzzy 模式），决定是否切换到下一个 key/端点/渠道。
4. 提供最小可观测能力：请求日志、失败率统计（用于健康检查/熔断/降级）。

## 影响范围
- **模块:** gateway/proxy、config、auth、scheduler、metrics、warmup(url)、session(affinity)
- **文件:** 新增 Go 服务源码与配置示例；更新知识库 API/架构文档
- **API:** 对外新增（或透传）`/v1/responses`、`/v1/models`、`/v1/*` 等
- **数据:** 以“内存指标”为主；可选扩展为 SQLite 持久化（后续任务可选）

## 核心场景

### 需求: 代理 /v1/responses（含 SSE 流式）
**模块:** proxy
对外提供与 OpenAI 兼容的 `/v1/responses` 转发能力。

#### 场景: 非流式请求
- 客户端携带代理访问密钥（`x-api-key` 或 `Authorization: Bearer`）发起请求
- 代理选择渠道→端点→key，成功返回 2xx JSON
- 上游返回不可重试错误时：不做 failover，原样返回错误

#### 场景: 流式请求（SSE）
- 上游返回 `Content-Type: text/event-stream` 时，代理应逐块转发并 flush
- 一旦开始向客户端写入响应头/正文，不再尝试 failover（避免半途重试导致语义错误）

### 需求: 代理 /v1/chat/completions（兼容 wire_api=chat）
**模块:** proxy
对外提供与 OpenAI 兼容的 `/v1/chat/completions` 转发能力（用于 Codex CLI/旧客户端兼容）。

#### 场景: 非流式请求
- 客户端以 `POST /v1/chat/completions` 发起请求并获得 2xx JSON
- 代理与 `/v1/responses` 共用同一套调度与 failover（在未写回前允许切换）

#### 场景: 流式请求（SSE）
- 上游以 `text/event-stream` 返回时，代理应逐块转发并 flush
- 一旦开始写回，禁止 failover（与 `/v1/responses` 一致）

### 需求: 代理 /v1/models
**模块:** proxy
对外提供 `/v1/models` 的转发能力，并复用同一套调度与 failover。

#### 场景: 列出模型
- 当首选渠道失败（网络错误/5xx/配额类错误）时可自动切换到下一个渠道/端点/key

### 需求: 多渠道优先级与自动切换
**模块:** scheduler
参考 `claude-proxy`：促销期 > Trace 亲和 > priority 顺序；并在失败时自动切换。

#### 场景: 按 priority 选择
- 有多个 active 渠道时，按 `priority` 从小到大尝试

#### 场景: 促销期优先
- 配置 `promotionUntil` 的渠道在有效期内优先（允许绕过健康检查）

#### 场景: Trace 亲和
- 同一用户/会话在 TTL 内优先命中上次成功渠道，减少跨渠道抖动

### 需求: 多 baseUrls 动态降级
**模块:** warmup/url_manager
同一渠道多个 `baseUrls`：根据运行时成功/失败反馈动态排序，失败端点冷却后再尝试。

### 需求: 多 key 冷却与降权
**模块:** config/metrics
- 失败 key 进入冷却期，短时间内不再优先尝试
- 配额类失败（402/429/余额/限流等）可将 key 移到队尾（软降级）

## 风险评估
- **风险:** 过度 failover 导致重复计费/语义不一致（尤其流式请求）  
  **缓解:** 仅在未开始写回响应前允许 failover；对不可重试错误严格禁止重试；默认读/写超时保护。
- **风险:** SSRF/内网探测（支持自定义 baseUrl 时）  
  **缓解:** 仅允许管理员配置上游地址；必要时可引入上游域名/网段白名单。
- **风险:** 日志泄漏敏感信息（API key、请求体）  
  **缓解:** 统一脱敏；默认不打印完整请求体；仅在开发模式允许截断输出。
