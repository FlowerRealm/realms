# API 手册

## 概述

本项目提供 OpenAI 兼容（Responses）与 Anthropic 兼容（Messages）的 API 入口，用于将客户端请求转发到上游（Codex OAuth 上游 + 自定义 baseUrl 的 OpenAI 兼容上游 + Anthropic 上游），并在中转层提供最小可用的调度与 SSE 透传能力。

## 认证方式

- **下游访问（调用本服务，多用户）:** `Authorization: Bearer <user_api_key>`（可选兼容 `x-api-key`）
- **上游访问（本服务调用上游）:**
  - **OpenAI 兼容上游:** API Key（由服务端配置并注入；baseUrl 仅管理员配置）
  - **Anthropic 上游:** API Key（由服务端配置并注入；使用 `x-api-key`；baseUrl 仅管理员配置）
  - **Codex OAuth 上游:** OAuth 凭据（导入/刷新后由服务端注入；严格脱敏）

Web 控制台使用 Cookie 会话（服务端会话表 + CSRF token）。

## 运行模式（self_mode）

- **默认模式（`self_mode.enable=false`）:** 完整功能（订阅/订单/充值/支付/工单等）可用；数据面按“订阅优先 + 余额兜底”进行配额与计费。
- **自用模式（`self_mode.enable=true`）:** 禁用计费/支付/工单相关路由与页面；数据面不再要求订阅/余额，仅记录 `usage_events` 供用量统计与排障。

## 功能禁用语义（feature_disable_*）

管理后台 `/admin/settings` 的「功能禁用」除了隐藏入口/路由 404 外，还会影响数据面语义：

- `feature_disable_billing=true`：数据面进入 free mode（不校验订阅/余额；仍记录 `usage_events`）
- `feature_disable_models=true`：关闭 `/models`、`/admin/models*`、`/v1/models`；并使数据面进入模型穿透（不要求模型启用/绑定；`model` 直接透传到上游；非 free mode 下仍要求模型定价存在）

---

## 接口列表（当前实现）

### realms（数据面）

#### [POST] /v1/responses
**描述:** OpenAI Responses create（支持 `stream=true` SSE 逐事件透传）。
**流式首包超时（failover）:**
- 当 `stream=true` 时，服务端在向下游写回任何内容之前，会等待上游首包输出；超时阈值由 `limits.stream_first_byte_timeout` / `REALMS_LIMITS_STREAM_FIRST_BYTE_TIMEOUT` 控制。
- 触发首包超时后会进行 failover（重新选渠/换 key/换账号），用于缓解“卡住不出字但连接未断”的情况；一旦开始写回则不会在中途切换上游。
**订阅/计费:**
- 默认模式：优先使用订阅额度；无订阅或订阅额度不足时，若启用“余额按量计费”则扣余额，否则返回 429 `订阅未激活/订阅额度不足`；余额不足返回 402 `余额不足`。
- free mode：当 `self_mode.enable=true` 或 `feature_disable_billing=true` 时，不检查订阅/余额；仅记录 `usage_events`（用于用量统计）。
**模型管理:**
- 默认：`model` 必须为管理员启用且存在可用渠道绑定的模型；服务会在允许的渠道集合内调度，并按选中渠道的 `upstream_model` 进行 alias 重写。
- 模型穿透（`feature_disable_models=true`）：
  - 不要求模型启用；非 free mode 下仍要求该 model 在 `managed_models` 中存在（用于计费口径）
  - 不要求存在渠道绑定；调度不再受 `channel_models` 白名单限制
  - `model` 字段直接透传到上游（不做 alias rewrite）
**请求字段策略（按渠道）:**
- 默认过滤 `service_tier` 与 `safety_identifier`
- `store` 默认透传；可在渠道级别禁用透传
- failover 到其他渠道时会重新按渠道策略处理
**字段转换（按渠道 /v1/responses）:**
- 推理力度后缀：支持 `-low/-medium/-high/-minimal/-none/-xhigh`，会写入 `reasoning.effort` 并从 `model` 移除后缀再转发
- `model_suffix_preserve` 命中时跳过后缀解析（保持 `model` 原样转发，不注入 `reasoning.effort`）
**请求体黑白名单（request_body_whitelist/request_body_blacklist，按渠道）:**
- 白名单：仅保留指定 JSON path 对应字段（不存在的 path 会被忽略）
- 黑名单：删除指定 JSON path 对应字段
- failover 到其他渠道时按新渠道重新应用（无跨渠道串扰）
**参数改写（param_override，按渠道）:**
- 管理员可为渠道配置 new-api 兼容 `operations`（JSON 路径操作）对请求体做改写
- 每次 selection 转发前应用；failover 到其他渠道时按新渠道重新应用
- 执行顺序：模型 alias rewrite →（Responses 模型后缀解析）→ 请求字段策略 → 请求体黑白名单 → param_override
  - 说明：请求字段策略/黑白名单用于过滤“用户透传字段”；`param_override` 作为管理员改写可重新设置这些字段（对齐 new-api 行为）
**Tokens 字段兼容:**
- `/v1/responses`：会将 `max_tokens/max_completion_tokens` 规范化为 `max_output_tokens`（优先对齐 OpenAI Responses API）
- 兜底兼容：当上游只接受 `max_tokens` 或只接受 `max_output_tokens` 时，Realms 会在收到 400 `Unsupported parameter` 后自动改写并重试一次
**请求头覆盖（header_override，按渠道）:**
- 管理员可为渠道配置请求头覆盖（JSON 对象），在转发到上游时生效
- 支持 `{api_key}` 变量替换（替换为该次请求实际选中的上游凭据）
- 注：默认鉴权在最后注入，`header_override` 无法覆盖 `Authorization`
**状态码映射（status_code_mapping，按渠道）:**
- 管理员可为渠道配置状态码映射（JSON 对象，如 `{"400":"200"}`）
- 仅改写对外返回的 HTTP status code；不影响 failover 判定与用量/审计口径

#### [POST] /v1/chat/completions
**描述:** OpenAI Chat Completions create 兼容端点（支持 `stream=true` SSE）。服务端会将 Chat 请求转换为 Responses 请求转发到上游，并将上游 Responses 响应转换回 Chat 格式返回。
**语义约束:**
- 该端点仅会调度到 `openai_compatible` 与 `codex_oauth` 类型渠道；不会将 Anthropic 上游映射为 Chat（避免协议差异导致的“看似成功但语义不一致”）。
- `messages[].content` 当前仅支持 `string` 与 `[{type:"text",text:"..."}]`；其他类型（如 `image_url`）会被忽略。
**订阅/计费、模型管理、渠道策略、failover:** 与 `/v1/responses` 一致（内部统一走 `/v1/responses` 作为计费与调度口径）。
**Tokens 字段兼容:**
- `/v1/chat/completions`：会使用 `max_tokens/max_completion_tokens` 作为转发到 `/v1/responses` 的 `max_output_tokens`
- 当请求体缺省 tokens 字段且服务端配置了默认值时，会补齐一个保守默认值（写入 `max_tokens`）

#### [POST] /v1/messages
**描述:** Anthropic Messages create（支持 `stream=true` SSE 逐事件透传）。
**语义约束:**
- 该端点仅会调度到 `anthropic` 类型渠道；不会将 OpenAI/Codex 上游映射为 Messages（避免协议差异导致的“看似成功但语义不一致”）。
- Anthropic 要求 `max_tokens`；当请求体缺省 `max_tokens` 且服务端配置了默认值时，会自动补齐一个保守默认值，避免上游拒绝请求。
**Tokens 字段兼容:**
- `/v1/messages`：当客户端误传 `max_output_tokens/max_completion_tokens` 时，会在转发前规范化为 `max_tokens`
**订阅/计费:** 与 `/v1/responses` 一致（free mode 同样仅记录 `usage_events`）。
**模型管理:** 与 `/v1/responses` 一致（支持模型穿透；非 free mode 下仍要求模型定价存在）。
**请求字段策略（按渠道）:** 与 `/v1/responses` 一致。
**请求体黑白名单（按渠道）:** 与 `/v1/responses` 一致。
**参数改写（param_override，按渠道）:** 与 `/v1/responses` 一致。
**请求头覆盖（header_override，按渠道）:** 与 `/v1/responses` 一致。
**状态码映射（status_code_mapping，按渠道）:** 与 `/v1/responses` 一致。

#### [GET] /v1/models
**描述:** OpenAI Models list（从本服务的模型目录输出，受管理员维护；仅返回存在可用渠道绑定的模型）。当 `feature_disable_models=true` 时该接口返回 404。

#### [GET] /api/usage/windows
**描述:** 查询当前用户在**指定时间区间**内的用量汇总（已结算/预留；单位 USD 小数），并附带请求数与 Token 统计（输入/输出/缓存输入/缓存输出/缓存比）。默认区间为**今天（UTC）**。
**认证:** Cookie Session（Web 登录会话）。
**参数:** `start`（YYYY-MM-DD，起始日期，UTC，默认今天）；`end`（YYYY-MM-DD，结束日期，UTC，默认与 start 相同；当 end=今天时区间结束时间取 now）。

#### [GET] /api/usage/events
**描述:** 查询当前用户的 `usage_events` 明细（分页，按 id 递减；仅返回已完成请求，默认过滤 `state=reserved` 的进行中记录）。除 `input_tokens/output_tokens` 与缓存 token 统计外，还会返回每次请求的 `endpoint/status_code/latency_ms/error_class/error_message/is_stream/request_bytes/response_bytes` 等字段，用于“按请求”展示用量与请求结果（不包含任何用户输入内容或模型输出全文）。其中当 `error_class=client_disconnect` 时不会对外输出该值（避免被误解为服务端错误）。
**认证:** Cookie Session（Web 登录会话）。
**参数:** `limit`（默认 100，最大 500）；`before_id`（向前翻页）；可选 `start/end`（YYYY-MM-DD，UTC，按时间区间过滤；不传则不做区间限制）。

#### [GET] /healthz
**描述:** 健康检查（含 DB 状态与构建信息：env/version/date）。

---

## Web 控制台（SPA，当前实现）

> 说明：Web 控制台为前后端分离的 SPA（`web/`）。后端仅负责静态资源/SPA 回落与 JSON API（`/api/*`）。
>
> UI 路由（GET；由 SPA 接管）：`/login`、`/register`、`/dashboard`、`/announcements*`、`/tokens*`、`/models`、`/usage`、`/account`、`/subscription`、`/topup`、`/pay/*`、`/tickets*`、`/admin/*`、`/oauth/authorize`。
>
> 自用模式提示：`self_mode.enable=true` 时，计费/支付/工单相关 API 不会注册（返回 404）。

### 会话与账号

#### [POST] /api/user/register
**描述:** 注册（JSON；成功后写入 Cookie 会话）。仅在 `security.allow_open_registration=true` 时开放。

#### [POST] /api/user/login
**描述:** 登录（JSON；成功后写入 Cookie 会话）。

#### [GET] /api/user/logout
**描述:** 登出并清理会话。

#### [GET] /api/user/self
**描述:** 获取当前登录用户信息。

### 邮箱验证码（注册用）

#### [POST] /api/email/verification/send
**描述:** 发送注册邮箱验证码（6 位数字码，10 分钟有效，HTML 邮件）。
**请求:** `application/x-www-form-urlencoded`：`email=<邮箱>`
**响应:** `{"sent":true}`

### 订阅/充值/支付（Billing）

#### [GET] /api/billing/subscription
**描述:** 订阅页数据（可购套餐 + 当前订阅 + 订单列表）。

#### [POST] /api/billing/subscription/purchase
**描述:** 创建订阅订单（JSON）。
**请求:** `{"plan_id":123}`

#### [GET] /api/billing/topup
**描述:** 充值页数据（余额 + 订单列表 + 可用支付渠道）。

#### [POST] /api/billing/topup/create
**描述:** 创建充值订单（JSON）。
**请求:** `{"amount_cny":"10.00"}`

#### [GET] /api/billing/pay/{kind}/{order_id}
**描述:** 支付页数据（订单信息 + 可用支付渠道）。`kind`：`subscription` / `topup`。

#### [POST] /api/billing/pay/{kind}/{order_id}/start
**描述:** 发起支付（JSON），返回支付跳转地址。
**请求:** `{"payment_channel_id":1,"epay_type":"alipay"}`
**响应:** `{"redirect_url":"https://..."}`（由前端执行跳转）

#### [POST] /api/billing/pay/{kind}/{order_id}/cancel
**描述:** 关闭订单（仅“待支付”可关闭；标记为“已取消”）。

### 支付回调（无需登录）

#### [POST] /api/webhooks/subscription-orders/{order_id}/paid
**描述:** 订阅订单 paid webhook（鉴权 + 幂等；用于无支付平台回调场景的兜底）。

#### [POST] /api/pay/stripe/webhook/{payment_channel_id}
**描述:** Stripe Webhook（按支付渠道验签 + 幂等入账/生效）。

#### [GET] /api/pay/epay/notify/{payment_channel_id}
**描述:** EPay notify 回调（按支付渠道验签 + 幂等入账/生效；按网关约定返回 `success`/`fail`）。

---

## OAuth Provider（外部应用授权，当前实现）

> 说明：用于外部客户端“跳转到 Realms → 用户登录 → 用户同意授权 → 回跳到客户端”，随后客户端用授权码交换得到 `rlm_...`，可直接调用 Realms 的 `/v1/*`。
>
> 安全边界：`redirect_uri` 由客户端传入，但必须与该应用登记的白名单 **精确匹配**；`state` 必填用于防 CSRF；授权码短期有效且一次性。

#### [GET] /oauth/authorize
**描述:** OAuth2 Authorization Code 授权入口（SPA 页面）。页面会调用后端 `/api/oauth/authorize` 完成参数校验与授权决策，并在需要时引导用户登录/同意授权。

#### [GET] /api/oauth/authorize
**描述:** 授权预检（Cookie Session；返回应用信息、scope、state，并在“已记住授权且 scope 相同”时直接返回 `redirect_to`）。
**参数:** `response_type=code`、`client_id`、`redirect_uri`、`state`（必填），可选 `scope`、`code_challenge`/`code_challenge_method`。
**响应:** `{"success":true,"message":"","data":{"app_name":"...","client_id":"...","redirect_uri":"...","scope":"...","state":"...","redirect_to":"可选"}}`

#### [POST] /api/oauth/authorize
**描述:** 授权决策（Cookie Session；JSON）。用户同意/拒绝后返回 `redirect_to`（由前端执行跳转）。
**请求:** `{"client_id":"...","redirect_uri":"...","scope":"...","state":"...","decision":"approve|deny","remember":true,"code_challenge":"可选","code_challenge_method":"可选"}`
**响应:** `{"success":true,"message":"","data":{"redirect_to":"..."}}`

#### [POST] /oauth/token
**描述:** 用授权码交换 Realms API Token（`rlm_...`）。
**请求:** `application/x-www-form-urlencoded`
**参数:** `grant_type=authorization_code`、`code`、`client_id`、`client_secret`、`redirect_uri`（可选 `code_verifier`；也支持 HTTP Basic 传 `client_id:client_secret`）。
**响应:** JSON：`access_token`（`rlm_...`）、`token_type=bearer`（可选 `scope`）。

---

## 管理面（SPA，当前实现）

> 说明：管理后台为 SPA（`/admin/*`），后端提供 JSON API（`/api/*`）；仅 `root` 会话可访问管理 API。
>
> 自用模式提示：`self_mode.enable=true` 时，计费/支付/工单相关管理接口不注册（返回 404）。

#### [GET] /admin/*
**描述:** 管理后台页面（SPA）。

### 管理 API（/api/admin/*，需要 root 会话）

- `GET /api/admin/home`：管理后台首页数据
- `GET /api/admin/settings` / `PUT /api/admin/settings` / `POST /api/admin/settings/reset`：系统设置（`app_settings`）
- `GET|POST|PUT|DELETE /api/admin/payment-channels`：支付渠道管理（`payment_channels`）
- `GET|POST|PUT|DELETE /api/admin/channel-groups`：分组管理（树形路由 SSOT）
- `GET|POST|PUT|DELETE /api/admin/users`：用户管理
- `GET|POST|PUT|DELETE /api/admin/announcements`：公告管理
- Billing：
  - `GET|POST|PUT|DELETE /api/admin/subscriptions`：订阅套餐管理
  - `GET /api/admin/orders` / `POST /api/admin/orders/{order_id}/approve|reject`：订阅订单管理
- `GET /api/admin/usage`、`GET /api/admin/usage/events/{event_id}/detail`：全局用量统计与明细
- `GET /api/admin/tickets` 等：工单管理（列表/详情/回复/关闭/恢复/附件下载）
- `GET|POST|PUT /api/admin/oauth-apps` 等：OAuth Apps 管理

### 其它 root API（不在 /api/admin 下）

- 上游渠道/端点/凭据/账号、渠道模型绑定等：`/api/channel/*`（root 会话）
- 模型目录管理：`/api/models/*`（root 会话）
