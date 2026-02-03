# 变更提案: codex（统一中转服务）

## 需求背景

当前仓库内已沉淀多份与“Codex/OpenAI API 中转”相关的方案与调研，但存在重复与分叉：
- `codex_proxy`：偏“通用 /v1/* 代理 + 多渠道 failover”
- `codex_responses_relay`：偏“Codex 上游（chatgpt.com/backend-api/codex）+ OAuth 账号池 + 企业治理”
- 调研包：Codex CLI wire API、claude-proxy 路由机制、new-api 的端口通信/SSE 中转实现要点等

本次整合目标是将上述内容收敛为一个新的方案包，并明确“最终只保留一个服务”，服务名称统一为 **codex**。

关键约束与偏好（来自需求确认）：
1. 最终产出一个新的方案包（`plan/` 下仅保留该包）。
2. 原有方案包全部归档到 `history/` 作为历史记录（不删除）。
3. 最终只保留一个服务：`codex`。
4. 同步修改索引文件（历史索引、知识库入口、变更记录）。

## 产品分析

### 目标用户与场景
- **用户群体:** 企业内部开发者/平台工程/工具链维护者；以及需要通过 Web 控制台管理接入与资源的管理员/普通用户
- **使用场景:** 内网/受限网络环境；多出口/多线路；多账号/多 key 容灾；Web 控制台自助 Token；分组（租户）隔离；统一鉴权与审计
- **核心痛点:** 单一上游或单一凭据波动导致不可用；SSE 流式中转不稳定导致体验崩坏；凭据与日志合规风险

### 价值主张与成功指标
- **价值主张:** 在不改变客户端调用方式的前提下，提供高可用、可配置、可观测、可治理的统一中转层
- **成功指标:**
  - 兼容性：`POST /v1/responses`（含 SSE）可用；`POST /v1/chat/completions` 可用；`GET /v1/models` 可用
  - 稳定性：在单渠道/单端点/单凭据失效时，能按策略自动切换（且不会在流式写回后“半途重试”）
  - 安全性：默认脱敏；凭据不明文落盘（或可控加密）；可审计但不采集输入内容

### 人文关怀
默认最小化敏感信息采集与持久化，避免在日志/指标/审计中泄漏用户输入与凭据；需要排障时提供“可控开关 + 采样 + 有效期清理”。

## 变更内容
1. 将现有方案与调研结论整合为一个新的 `codex` 方案包（why/how/task）。
2. 规划一个单体服务 `codex`：对外提供 OpenAI 风格 API（重点 `responses/models/chat`），并具备稳定的 SSE 中转能力。
3. 统一“多渠道/多端点/多凭据”调度与 failover 策略（参考 claude-proxy 的三层模型）。
4. 纳入 Codex OAuth 账号池能力：支持导入/刷新/轮询，并与自定义 baseUrl 的 OpenAI 兼容上游并存（同一服务统一调度）。
5. 引入用户体系与 Web 控制台：注册/登录、一个用户多个 Token、分组隔离、配额（套餐/订阅）对接点与审计。
6. 更新知识库索引：确保新的方案入口清晰、旧包可追溯。

## 影响范围
- **模块:** `codex`（新服务，计划为 Go 实现）
- **文件:**
  - 新增 `helloagents/plan/*_codex/` 方案包文件
  - 归档旧方案包至 `helloagents/history/`
  - 更新 `helloagents/wiki/*`、`helloagents/history/index.md`、`helloagents/CHANGELOG.md`
- **API:** 对外计划支持 `POST /v1/responses`、`POST /v1/chat/completions`、`GET /v1/models`；可选通用 `/v1/*` 代理
- **数据:** 运行态指标/冷却/亲和会话；MySQL 持久化（用户/凭据/审计/配置，敏感字段加密）

## 核心场景

### 需求: OpenAI Responses API 兼容（含 SSE）
**模块:** codex（数据面）
对外提供 `POST /v1/responses`，`stream=true` 时稳定透传 `text/event-stream`。

#### 场景: SSE 流式响应
- 事件边界保持：逐条转发并 flush，避免“读完再回写”
- 断连/取消传播：客户端断开后应尽快取消上游请求，避免资源泄漏
- failover 边界：一旦开始写回响应（header/body），禁止再切换上游（避免语义与计费问题）

#### 场景: 非流式响应
- 返回符合 Responses response object 的 JSON
- 错误分类可控：不可重试错误不做无意义重试

### 需求: Chat Completions 兼容（用于 wire_api=chat）
**模块:** codex（数据面）
当客户端使用 `wire_api=\"chat\"` 时，提供 `POST /v1/chat/completions`。

### 需求: Models API
**模块:** codex（registry）
提供 `GET /v1/models`，并支持 alias/过滤（可选）。

### 需求: Web 控制台（注册/登录/Token 管理）
**模块:** codex（web/auth/token）
提供 Web UI（服务端渲染）用于用户注册/登录与 Token 自助管理；不做 2FA/OAuth。

#### 场景: 开发期开放注册
- 开发期允许注册（暂不强制邮箱验证）
- 邮件能力实现后切换为“注册强制邮箱验证”

#### 场景: 一个用户多个 Token
- 用户可创建多个 Token 用于不同服务/环境
- Token 可随时撤销，撤销后立即失效

### 需求: 分组隔离（租户隔离）
**模块:** codex（auth/router/store）
用户/Token/上游资源按组隔离，默认不跨组 fallback。

#### 场景: 组内资源可用性约束
- 调度/路由仅在组内允许的渠道/端点/凭据集合中选择
- 跨组访问与跨组重试默认拒绝

### 需求: 配额来自套餐/订阅（对接）
**模块:** codex（quota/store）
配额来自套餐/订阅；套餐逻辑仍在实现中，本方案提供对接点与最小可用口径。

**已确认口径:**
- 计量: 成本（`usd_micros`）
- 定价表: 管理员维护/控制
- 时间窗: rolling 5h / rolling 7d / rolling 30d（相对时间）
- 订阅周期: 相对月（按购买时间 + 1 month）
- 绑定: 用户账号（`user_id`，来源于数据库）
- 叠加: 并行（多订阅额度汇总）
- 超额: 直接拒绝（提示“余额不足”，无需返回额外字段）

#### 场景: 配额不足拒绝请求
- 数据面请求前置校验配额，不足则返回统一错误体

### 需求: 多渠道/多端点/多凭据的自动切换
**模块:** scheduler / metrics
采用三层 failover：Channel → Endpoint(BaseURL) → Credential(Key/Account)，并提供冷却/熔断保护。

### 需求: Codex OAuth 账号接入与刷新
**模块:** auth / store
支持导入 `~/.codex/auth.json` 或等价凭据并自动刷新；账号轮询与隔离。

### 需求: Codex OAuth 会话粘性绑定与 RPM 负载均衡
**模块:** router / scheduler
基于 routeKey（`prompt_cache_key` > `Conversation_id` > `Session_id` > `Idempotency-Key`）做会话绑定（TTL=30min，临时存储）；绑定渠道失败时先重试 3 次（所有错误都重试），仍失败才重绑到其他可用账号，并按 rolling RPM（1m 窗口）选择负载最低者；SSE/流式一旦开始写回后禁止 failover。

## 风险评估

- **风险:** 将“非公开/非稳定”的上游形态（如 chatgpt.com/backend-api/codex）纳入生产链路，存在合规与稳定性风险  
  **缓解:** 默认以官方公开 API 为主路径；若启用 OAuth/非公开上游，要求明确授权与隔离边界，并提供开关与降级策略。
- **风险:** SSE 流式在中间层被压缩/缓冲导致“假流式”  
  **缓解:** 明确禁止对 SSE 响应 gzip；逐事件 flush；必要时加入 ping 保活（可选）。
- **风险:** SSRF/内网探测（支持自定义 baseUrl 时）  
  **缓解:** 仅允许管理员配置上游地址；严格校验重定向与代理配置。
- **风险:** 日志/审计泄漏敏感信息（token/key/输入内容）  
  **缓解:** 默认脱敏与最小采集；仅记录索引字段；提供可控的排障采样开关。
- **风险:** Web 控制台暴露扩大攻击面（弱口令、会话劫持、CSRF）  
  **缓解:** 密码安全哈希、会话过期与登出、CSRF 防护、登录/注册限速、严格审计；管理入口可内网/独立域名隔离。
- **风险:** 开发期“无邮箱验证注册”被误用于生产  
  **缓解:** 明确环境开关；生产默认禁用开放注册或强制邮箱验证。
- **风险:** 分组隔离配置错误导致越权使用上游资源  
  **缓解:** 默认拒绝跨组；管理变更强审计；配置校验与回滚策略。
