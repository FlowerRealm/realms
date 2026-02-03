# 任务清单: codex（统一中转服务）

目录: `helloagents/history/2026-01/202601131914_codex/`

---

## 0. 决策确认（阻塞项）
- [√] 0.1 已确认上游策略：`codex_oauth` + `openai_compatible`（自定义 baseUrl 的 OpenAI 格式请求）
- [√] 0.2 已确认存储：MySQL（敏感字段应用层加密）
- [√] 0.3 已确认北向协议：同时支持 `responses` 与 `chat`（并保留 `GET /v1/models`）
- [√] 0.4 已确认下游体系：Web 控制台 + 多用户 + 多 Token（数据面鉴权）
- [√] 0.5 已确认护栏：对外商用场景需最小保护（单实例：并发/连接/超时/体积/默认 max_output_tokens），不做分布式限流
- [√] 0.6 已确认分组隔离：用户/Token/上游资源按组隔离，默认不跨组 fallback
- [√] 0.7 已确认 Web UI：服务端渲染（SSR），不做 2FA/OAuth
- [√] 0.8 已确认注册策略：开发期开放注册；邮件能力完成后再强制邮箱验证
- [√] 0.9 已确认配额来源：来自“套餐/订阅”（套餐逻辑实现中，本期先预留对接点）
- [√] 0.10 已确认配额口径：计量=成本（`usd_micros`）+ 管理员定价表；窗口=rolling 5h/7d/30d（相对时间）；订阅=相对月；`user_id` 来自数据库；叠加=并行；超额=直接拒绝（“余额不足”）

## 1. 项目骨架（Go 服务）
- [√] 1.1 初始化 Go 模块与基础目录结构（`cmd/codex`、`internal/*`），提供最小启动入口
- [√] 1.2 实现配置加载（文件 + 环境变量覆盖），并支持从 MySQL 加载上游 `channel/endpoint/credential`（`codex_oauth`/`openai_compatible` 同级）
- [√] 1.3 增加 `GET /healthz`（含版本信息），并输出当前配置摘要（脱敏）

## 2. 用户体系、鉴权与基础中间件
- [√] 2.1 设计并创建 MySQL schema/migrations：`users`、`groups`、`user_tokens`、`user_sessions`、`email_verifications`（预留）、`upstream_channels`、`upstream_endpoints`、`openai_compatible_credentials`、`codex_oauth_accounts`、`audit_events`、`pricing_models`（管理员定价表）、`usage_events`（用量/计费事件）
- [√] 2.2 实现 Web 账号体系：注册/登录/登出/会话（开发期允许无邮箱验证；邮件能力完成后强制）
- [-] 2.3 实现 Web 控制台（SSR）：Token 管理、配额/用量概览、管理员后台（用户/分组/审计/上游配置）
  > 备注: 已实现 SSR 注册/登录/会话 + Token 管理 + 上游配置管理后台；配额/用量概览、用户/分组/审计全量后台未完成。
- [-] 2.4 实现上游配置管理（admin）：维护 `openai_compatible` 的 channels/endpoints/keys（必须与 `group_id` 绑定；baseUrl 必须通过 SSRF 校验；key 加密存储、可撤销）
  > 备注: 已实现创建/列表 + SSRF 校验 + 加密入库；禁用/撤销 UI 与审计写入未完成。
- [-] 2.5 实现上游配置管理（admin）：维护 `codex_oauth` 的 channels/endpoints/accounts（导入/刷新/禁用；与 `group_id` 绑定；导入/刷新按 `endpoint_id` 归属）
  > 备注: 已实现账号手工录入与加密入库 + 列表；导入/刷新/禁用未完成。
- [√] 2.6 实现数据面 Token 鉴权：`Authorization: Bearer <token>` 与可选 `x-api-key`（MySQL 查 token hash），写入 `user_id/token_id/group_id`
- [√] 2.7 实现分组隔离约束：请求上下文携带 `group_id`，调度/路由仅使用组内资源（默认不跨组）
- [-] 2.8 预留套餐/订阅配额对接点：定义 `QuotaProvider`（成本 `usd_micros` + rolling window；定价表管理员维护）并在数据面请求前置校验/预留配额（按套餐配置的 window 需要预估/预留，避免“最后一笔穿透”）
  > 备注: 已实现 `QuotaProvider` + usage_events 预留/结算/作废/过期；未接入套餐/订阅额度校验（本期默认不限制或按后续扩展实现）。
- [-] 2.9 明确“上游未计费=本地不计费”的判定规则：无 usage 且未写回任何内容→计费=0；已开始写回→按已输出内容/usage 结算
  > 备注: 已实现基础口径（失败未写回→void；成功/开始写回→commit=0或usage）；流式 usage/本地估算未完成。
- [-] 2.10 加入请求体大小限制（含解压后限制）与超时控制（read/header/write）
  > 备注: 已实现请求体大小限制（未包含解压后限制）+ server 超时 + 每请求超时中间件。
- [√] 2.11 实现 request_id 生成/透传（header 与日志字段）
- [√] 2.12 实现 body 可复用缓存（支持“解析/校验 → 转发/重试”多次读取）
- [-] 2.13 实现最小权限边界：普通用户仅自身；group admin 仅本组；root 可跨组（所有管理操作写审计）
  > 备注: 已实现用户自身 Token 管理；管理后台仅 root/group_admin；root 可指定 group_id 创建 channel；审计写入未全面接入。
- [X] 2.14 实现定价表管理（admin）：`pricing_models` 的 CRUD、模型通配匹配规则与倍率（ppm/decimal）校验
  > 备注: 已实现表结构与基础查询/创建接口（store 层），未实现管理后台与完整校验规则。
- [√] 2.15 实现用量事件闭环：`usage_events` 的 reserved→committed→void/expire 状态机 + 超时清理任务（防止预留卡死）

## 3. 调度与 failover（Channel → Endpoint → Credential）
- [√] 3.1 实现 ChannelScheduler：促销期 > Trace 亲和 > priority > fallback（失败率最低）
- [√] 3.2 实现 TraceAffinity（TTL）：基于鉴权后的 `user_id`（而非从请求体推断）
- [X] 3.3 实现 URLManager：多 baseUrls 动态排序 + 冷却（按请求结果反馈）
  > 备注: 当前仅按 endpoint priority 选择，未实现动态排序与冷却。
- [-] 3.4 实现 Credential 冷却与熔断：滑窗失败率 + 最小样本保护 + 配额类失败降权
  > 备注: 已实现最小冷却（retriable 失败触发 cooldown）；滑窗失败率/最小样本/配额类降权未完成。
- [-] 3.5 实现错误分类策略（Normal + Fuzzy），明确不可重试错误清单
  > 备注: 已实现基础 retriable status 分类；完整错误分类与不可重试清单未完成。
- [√] 3.6 将 `group_id` 纳入调度选择约束：仅在组内可用渠道/端点/凭据集合中选择
- [√] 3.7 实现 routeKey 提取（优先级固定）：`prompt_cache_key` > `Conversation_id` > `Session_id` > `Idempotency-Key`
- [√] 3.8 实现会话粘性绑定（TTL=30min，临时存储）：`group_id:route_key_hash` → `credential_id`，命中成功后 touch 续期
- [√] 3.9 实现 rolling RPM（窗口 60s）统计，并用于新会话/重绑选号（选择 RPM 最低且可用的 credential）
- [-] 3.10 实现“重试→重绑”：绑定 credential 失败先重试 3 次（所有错误都重试），仍失败才切换并更新绑定；同请求限制总尝试/切换次数，避免死循环
  > 备注: 已实现“绑定命中→重试 3 次→清理绑定→重新选择”；全错误覆盖/尝试次数精细控制未完全对齐。
- [√] 3.11 单实例运行态状态默认仅内存：SessionBinding/RPM/Cooldown/熔断/亲和不落 MySQL（多实例再迁移 Redis/外置存储）

## 4. 北向 API（OpenAI 兼容）
- [√] 4.1 实现 `POST /v1/responses`：stream=true/false；覆盖 why.md#需求-openai-responses-api-兼容（含-sse）
- [√] 4.2 实现 SSE 直通：检测 `text/event-stream` 并按事件边界转发 + flush；开始写回后禁止 failover
- [-] 4.3 实现 `GET /v1/models`：支持基础列表与可选 alias/过滤
  > 备注: 当前实现为上游透传，未做 alias/过滤。
- [√] 4.4 实现 `POST /v1/chat/completions`：用于 `wire_api=chat` 兼容（含 SSE）
- [-] 4.5 （可选）实现通用 `ANY /v1/*` 代理：谨慎处理未知端点与不可重试错误
  > 备注: 本期未启用通用代理端点。
- [-] 4.6 计费信号采集：优先读取上游 `usage`；缺失则本地估算（按已输出文本/事件累计）；流式开始写回后客户端断开仍结算
  > 备注: 非流式支持从响应体提取 usage（常见字段）；缺失时按 0 结算；流式 usage/本地估算未完成。

## 5. Upstream 执行器（上游调用）
- [√] 5.1 实现上游类型 `openai_compatible`：按 baseUrl 构建目标 URL，从 `openai_compatible_credentials` 取 key 注入鉴权，透传请求头/体
- [-] 5.2 实现上游类型 `codex_oauth`：构造 `chatgpt.com/backend-api/codex` 请求并注入必要 headers（严格脱敏日志）
  > 备注: 已实现基础路径映射与必要 header 注入；完整 header 兼容与刷新/轮询仍需补齐。
- [√] 5.3 实现上游请求取消传播：下游断开/超时触发上游 cancel
- [-] 5.4 实现上游重试上限与退避（仅在未写回前；避免放大故障）
  > 备注: 已实现 failover 重试（无退避策略）；指数退避与放大保护未完成。
- [X] 5.5 对 `openai_compatible` 尽量启用 stream usage（如上游支持）以精确计费；不支持/缺失 usage 时走本地估算兜底
  > 备注: 本期未接入 stream usage。

## 6. OAuth 账号池（Codex OAuth 上游）
- [X] 6.1 实现凭据导入接口（导入 `~/.codex/auth.json` 或等价结构），覆盖 why.md#需求-codex-oauth-账号接入与刷新
  > 备注: 本期未实现导入接口。
- [X] 6.2 实现 token 刷新服务（singleflight/分布式锁），并做“提前刷新 + 过期兜底”
  > 备注: 本期未实现刷新服务。
- [-] 6.3 实现账号状态存储与加密（MySQL + 应用层加密）
  > 备注: 已实现表结构与加密入库；与刷新/轮询联动的状态机未完成。
- [X] 6.4 实现多账号轮询与熔断：round-robin + 可用性过滤 + 冷却退避 + 恢复探测
  > 备注: 本期未实现完整账号池轮询/恢复探测。

## 7. 护栏与配额保护（MVP 必做，单实例）
- [√] 7.1 per-token 并发上限 + SSE 连接上限（超限直接拒绝，避免打爆机器与上游资源）
- [√] 7.2 默认最大输出（`max_output_tokens/max_tokens`）与最大请求时长（超时取消上游），防止“无限长流式”穿透套餐 window
- [√] 7.3 per-upstream-credential 并发保护（避免单账号/单 key 被打爆；与 rolling RPM 指标联动）

## 8. 安全与可观测
- [-] 8.1 实现日志脱敏中间件（headers/body 规则），并增加关键结构化字段（request_id/user_id/model/channel）
  > 备注: 已实现最小 access log（结构化字段）且不记录敏感 header/body；完整脱敏规则与审计字段仍需补齐。
- [X] 8.2 增加最小指标：QPS、p95/p99、429/5xx 比例、冷却数量、刷新失败率（按启用能力裁剪）
  > 备注: 本期未实现指标端点/采集。
- [-] 8.3 （可选）接入 OpenTelemetry tracing（含 upstream span）
  > 备注: 本期未接入。
- [-] 8.4 主密钥管理：上游 key/token 加密存储；主密钥仅从环境/密钥管理注入；支持 key_id 轮换；严禁写日志
  > 备注: 已实现 AES-GCM 加密入库与 dev 环境临时 key 警告；key_id 轮换未实现。

## 9. 文档更新（知识库同步）
- [√] 9.1 更新 `helloagents/wiki/overview.md`：补充 codex 模块与关键入口
- [√] 9.2 更新 `helloagents/wiki/arch.md`：补充 `codex` 架构图与 ADR 索引入口
- [√] 9.3 更新 `helloagents/wiki/api.md`：补充对外代理接口与鉴权说明
- [√] 9.4 更新 `helloagents/wiki/data.md`：补充运行态/存储数据模型
- [√] 9.5 更新 `helloagents/wiki/modules/codex.md`：增加“中转服务”视角的模块说明与变更历史

## 10. 安全检查
- [-] 10.1 执行安全检查（按G9：输入验证、敏感信息处理、权限控制、SSRF 防护、EHRB 风险规避）
  > 备注: 已做 SSRF 校验、敏感字段加密与日志最小化；完整安全清单需进一步审计。

## 11. 测试
- [√] 11.1 添加调度器单测：promotion/affinity/priority/fallback 的选择顺序
- [√] 11.2 添加 failover 集成测试：credential→url→channel 切换；覆盖 5xx/429/网络错误场景
- [√] 11.3 添加 SSE 透传测试：event-stream 逐条转发且不阻塞（覆盖 `/v1/responses` 与 `/v1/chat/completions`）
- [√] 11.4 添加脱敏测试：确保 token/key 不出现在日志与审计输出

## 12. 部署
- [√] 12.1 提供配置示例与运行说明（本地/容器）
- [-] 12.2 （可选）提供云部署清单（K8s/Helm 或等价）
  > 备注: 本期未提供 K8s/Helm 清单。
