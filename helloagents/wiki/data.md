# 数据模型

## 概述

本文件描述 realms 的数据模型（MVP）。**最终以代码与迁移文件为准**：

- `internal/store/migrations/*.sql`

## 时间口径（UTC）

- 所有数据库 `DATETIME` 字段以 **UTC** 口径存储与查询（应用侧统一用 UTC 计算统计区间；展示时再按需要转为本地时区）。
- MySQL 连接会强制设置会话时区为 `time_zone='+00:00'`，并以 `loc=UTC` 解析/编码 `DATE/DATETIME`（见 `internal/store/db.go`）。
- 管理后台展示时区可通过 `app_settings.admin_time_zone` 配置（默认 `Asia/Shanghai`）。

## 运行态（内存）模型（草案）

- `Binding`
  - `user_id + route_key_hash` → `selection`（TTL；会话粘性）
- `Affinity`
  - `user_id` → `channel_id`（TTL；亲和）
- `RPM`
  - `credential_key` → `recent_timestamps[]`（滑动窗口请求计数；窗口=60s 时等价 rolling RPM，用于负载均衡/节流参考）
- `ChannelRPM`
  - `channel_id` → `recent_timestamps[]`（历史：用于 channel 维度 RPM 限额；当前实现已移除）
- `CredentialCooling`
  - `credential_key` → `cooldown_until`（可重试失败后的凭证冷却）
- `ChannelFailScore`
  - `channel_id` → `fail_count`（失败评分，用于排序倾向）
- `ChannelBan`
  - `channel_id` → `fail_streak + ban_until`（渠道级自动禁用；失败次数递增时延长 ban 时间；成功清零）

## 凭据与账号（可选持久化）

> 已确认：持久化支持 MySQL/SQLite（SQLite 为单机自举）。用户侧 Token/Session 仅存 hash；上游 credential/account 明文入库（依赖数据库权限、备份/磁盘/云盘加密等措施）。

## MySQL/SQLite（当前实现）

> 当前实现仍为**单租户**：不存在租户级 `group_id`；但引入“分组”（`user_groups` / `upstream_channels.groups`）用于按用户组集合筛选可用上游渠道（仅 root 可管理）。

### 用户/鉴权

- `users`：用户（email + bcrypt 密码哈希 + role）
- `user_groups`：用户分组（多对多；用户可加入多个组；强制包含 `default`）
  - `user_id + group_name` 联合主键
- `channel_groups`：分组字典（用于下拉选择/统一管理）
  - `name`：分组名（唯一；建议只用字母/数字及 `_ -`）
  - `status`：启用/禁用（禁用后不建议继续分配给用户/渠道）
  - `description`：描述（可选）
  - `price_multiplier`：分组价格倍率（小数；**订阅**计费成本=模型单价×倍率；按量计费余额不使用该倍率；默认 `1.0`）
  - `max_attempts`：组内最大尝试次数（用于“渠道组树形路由”；默认 `5`）
  - 删除语义：强制删除并清理引用（解绑 `user_groups` 与 `upstream_channels.groups`；若某渠道仅属于该分组，删除时会自动禁用该渠道并回退到 `default`）
- `channel_group_members`：渠道组成员关系（树形路由 SSOT）
  - `parent_group_id`：父组（`channel_groups.id`）
  - `member_group_id`：子组（可空；组节点**单父**）
  - `member_channel_id`：叶子渠道（可空；渠道允许被多个组引用）
  - `priority/promotion`：成员排序字段（promotion 优先，其次 priority）
  - 约束：`member_group_id` 全局唯一（实现“子组只能有一个父组”）；`(parent_group_id, member_group_id)` 与 `(parent_group_id, member_channel_id)` 去重
  - SQLite 自举：启动时会根据 `upstream_channels.groups` 回填 `channel_group_members`（幂等），避免 default 根组无成员导致路由不可用
- `user_tokens`：数据面 Token（仅存 hash；可撤销/可删除）
- `user_sessions`：Web 会话（仅存 hash；含 csrf_token）
- `email_verifications`：邮箱验证码（注册邮箱验证；6 位数字码；10 分钟有效；仅存 hash；`user_id` 可为空）
- `app_settings`：应用级可变配置（少量运行期开关；可由管理后台写入并持久化）
  - `email_verification_enable`：是否启用注册邮箱验证码（bool；DB 覆盖配置文件默认）
  - `smtp_server`：SMTPServer（string；DB 覆盖配置文件默认）
  - `smtp_port`：SMTPPort（int；DB 覆盖配置文件默认）
  - `smtp_ssl_enabled`：SMTPSSLEnabled（bool；DB 覆盖配置文件默认）
  - `smtp_account`：SMTPAccount（string；DB 覆盖配置文件默认）
  - `smtp_from`：SMTPFrom（string；DB 覆盖配置文件默认）
  - `smtp_token`：SMTPToken（string；敏感信息；不回显；DB 覆盖配置文件默认）
  - `billing_enable_pay_as_you_go`：是否启用余额按量计费（bool；DB 覆盖配置文件默认）
  - `billing_min_topup_cny`：最小充值金额（CNY；最多 2 位小数；DB 覆盖配置文件默认）
  - `billing_credit_usd_per_cny`：充值入账比例（每 1 CNY 增加多少 USD；最多 6 位小数；DB 覆盖配置文件默认）
  - `feature_disable_web_announcements`：禁用 Web 公告（bool；true=禁用；缺省=启用）
  - `feature_disable_web_tokens`：禁用 Web Token 管理页（bool；true=禁用；缺省=启用）
  - `feature_disable_web_usage`：禁用 Web 用量统计页与 `/api/usage/*`（bool；true=禁用；缺省=启用）
  - `feature_disable_models`：禁用模型域（bool；true=禁用；缺省=启用；同时使数据面进入模型穿透）
  - `feature_disable_billing`：禁用计费/支付/订阅/充值（bool；true=禁用；缺省=启用）
  - `feature_disable_tickets`：禁用工单（bool；true=禁用；缺省=启用）
  - `feature_disable_admin_channels`：禁用管理后台上游渠道相关页面（bool；true=禁用；缺省=启用）
  - `feature_disable_admin_channel_groups`：禁用管理后台分组页面（bool；true=禁用；缺省=启用）
  - `feature_disable_admin_users`：禁用管理后台用户管理页（bool；true=禁用；缺省=启用）
  - `feature_disable_admin_usage`：禁用管理后台用量统计页（bool；true=禁用；缺省=启用）
  - `feature_disable_admin_announcements`：禁用管理后台公告页（bool；true=禁用；缺省=启用）
  - 注：支付渠道配置不在 `app_settings` 中维护，统一由 `payment_channels` 表承载。

### 工单系统（用户支持）

- `tickets`：工单（用户提交的问题/反馈）
  - `user_id`：创建者
  - `status`：工单状态（打开/关闭）
  - `last_message_at`：最后一次消息时间（用于列表排序）
  - `closed_at`：关闭时间（NULL 表示未关闭）
- `ticket_messages`：工单消息线程（用户/管理员往返沟通）
  - `actor_type`：`user` / `admin` / `system`
  - `actor_user_id`：消息发送者（可为空）
  - `body`：消息正文（TEXT）
- `ticket_attachments`：工单附件元信息（文件本体落盘到本地目录）
  - `storage_rel_path`：相对路径（由服务端生成，防路径穿越）
  - `expires_at`：过期时间（默认 7 天）

> 附件清理：服务端后台会定时删除 `expires_at < NOW()` 的附件文件与记录（best-effort），避免磁盘无限增长。

### 公告系统

- `announcements`：公告（管理员发布，用户只读）
  - `status`：0=草稿，1=已发布（用户侧仅展示已发布）
  - `title/body`：标题与正文（正文为 TEXT，前端以 `pre-wrap` 展示以保留换行）
- `announcement_reads`：用户已读标记（联合主键 `user_id + announcement_id`）
  - `read_at`：阅读时间；写入幂等（重复标记不会覆盖）

### 上游资源

- `upstream_channels`：上游渠道（`openai_compatible` / `codex_oauth`，全局）
  - `groups`：渠道所属分组（逗号分隔多个分组；默认 `default`；作为兼容缓存，受 `channel_group_members` 变更回填更新）
  - `allow_service_tier`：是否允许 `service_tier` 透传到上游（默认过滤，避免额外计费）
  - `disable_store`：是否禁用 `store` 透传到上游（默认允许，禁用可能影响 Codex/Caching 类功能）
  - `allow_safety_identifier`：是否允许 `safety_identifier` 透传到上游（默认过滤，保护用户隐私）
  - `param_override`：按渠道请求体参数改写（new-api `operations` 兼容）
  - `header_override`：按渠道请求头覆盖（支持 `{api_key}` 变量替换）
  - `status_code_mapping`：按渠道状态码映射（仅改写对外 HTTP status code）
  - `model_suffix_preserve`：模型后缀保护名单（JSON 数组；命中时跳过 `/v1/responses` 的推理后缀解析）
  - `request_body_whitelist`：请求体白名单（JSON 数组；按 JSON path 仅保留指定字段）
  - `request_body_blacklist`：请求体黑名单（JSON 数组；按 JSON path 删除指定字段）
  - `last_test_at`：最近一次测试时间（NULL 表示从未测试）
  - `last_test_ok`：最近一次测试是否成功（1/0）
  - `last_test_latency_ms`：最近一次测试延迟（毫秒）
- `upstream_endpoints`：上游端点（base_url；**每个 channel 固定 1 条**）
- `openai_compatible_credentials`：API key（明文入库）
- `codex_oauth_accounts`：OAuth token（明文入库）
- `codex_oauth_pending`：Codex OAuth 授权 pending state（短期缓存，用于回调校验与换取 token；支持“粘贴回调 URL”、进程重启与多实例）

### OAuth Apps（外部客户端授权）

- `oauth_apps`：OAuth 应用（`client_id/name/status/client_secret_hash`）
- `oauth_app_redirect_uris`：应用回调地址白名单（`redirect_uri` 精确匹配；`redirect_uri_hash` 用于唯一索引，避免长 URI 触发索引长度限制）
- `oauth_user_grants`：用户授权记录（用于“记住授权/跳过重复确认”）
- `oauth_auth_codes`：授权码（只存 hash；短期有效；单次消费）
- `oauth_app_tokens`：OAuth 发放 Token 与应用/用户的映射（用于按应用撤销/审计扩展）

> sessions 口径：使用数据面请求携带的 `route_key_hash`（由 `Conversation-Id/Session-Id/...` 等头或 payload 的 `prompt_cache_key` 计算）进行会话粘性绑定；某账号当前被多少个未过期 binding 占用，即视为该账号占用多少 sessions。

### 模型目录

- `managed_models`：模型目录（白名单/展示元信息；对外 `GET /v1/models` 与数据面 `model` 校验的 SSOT）
  - `public_id`：对外模型名（客户端请求使用）
  - `owned_by`：仅用于展示
  - `input_usd_per_1m/output_usd_per_1m/cache_input_usd_per_1m/cache_output_usd_per_1m`：按模型定价（单位：USD / 1M tokens；小数）
  - `status`：模型全局启用/禁用
- `channel_models`：渠道绑定模型（channel → models；alias 与上游路由以此为准）
  - `channel_id + public_id`：唯一
  - `upstream_model`：该 channel 上游真实模型名（alias → upstream）
  - `status`：绑定启用/禁用

> 可用模型判定：`managed_models.status=1` 且存在至少一个 `channel_models.status=1` 且对应 `upstream_channels.status=1` 的绑定。

### 审计/计费

- `audit_events`：审计索引（不记录输入内容与明文凭据）
- `usage_events`：用量事件（reserved → committed → void/expired；记录 input/output tokens，并支持 cached_input_tokens/cached_output_tokens 统计；`subscription_id` 用于将用量归属到具体订阅；`upstream_channel_id` 用于按渠道统计用量与成本；并记录 `endpoint/status_code/latency_ms/error_class/error_message/is_stream/request_bytes/response_bytes` 用于按每次请求展示明细——不落库任何用户输入内容或模型输出全文）
  - 计费口径：仅使用上游返回的 usage tokens（含 cached tokens），结合 `managed_models` 的单价在本地计算成本；不读取上游响应中的“金额/费用”等字段。
  - 按量计费（`subscription_id IS NULL`）：预留阶段先从 `user_balances.usd` 扣减 `reserved_usd`；结算阶段写入 `committed_usd` 并执行余额返还/补扣：
    - `committed_usd < reserved_usd`：返还差额
    - `committed_usd > reserved_usd`：尝试补扣差额（余额不足时最多扣到 `0`，并将实际扣到的金额写入 `committed_usd`）
  - 订阅计费（`subscription_id IS NOT NULL`）：不扣 `user_balances`，并按订阅套餐所属分组的 `channel_groups.price_multiplier` 计算/记录成本。
- `payment_channels`：支付渠道（`stripe` / `epay`；每个渠道一份独立配置；用于支付页选择与按渠道回调验签）
  - `type/name/status`：类型/名称/启用状态
  - `stripe_currency/stripe_secret_key/stripe_webhook_secret`：Stripe 配置
  - `epay_gateway/epay_partner_id/epay_key`：EPay 配置
- `user_balances`：用户余额（按量计费余额；单位 USD 小数；充值/预留/结算会更新该表；订阅计费不会修改此表）
- `topup_orders`：充值订单（支付成功后增加 `user_balances.usd`）
  - `amount_cny/credit_usd`：充值金额（CNY）/入账额度（USD）
  - `status`：0=待支付，1=已入账，2=已取消
  - `paid_at/paid_method/paid_ref/paid_channel_id`：支付信息（best-effort）
- `subscription_plans`：订阅套餐（价格 + 额度窗口）
  - `group_name`：订阅套餐所属分组（默认 `default`；订阅页仅展示用户所属组内套餐；购买时服务端二次校验用户属于该组）
  - `price_cny`：价格（CNY）
  - `duration_days`：订阅有效期（天）
  - `limit_5h_usd/limit_1d_usd/limit_7d_usd/limit_30d_usd`：滚动窗口额度（按 USD 成本估算后扣减；`<=0` 表示该窗口不限额）
- `subscription_orders`：订阅订单（购买先下单；支付/批准后生效并创建 `user_subscriptions`）
  - `status`：0=待支付，1=已生效，2=已取消（预留）
  - `paid_at/paid_method/paid_ref/paid_channel_id`：支付信息（best-effort）
  - `approved_at/approved_by`：管理员批准信息（兜底；仅 `root` 可操作）
  - `subscription_id`：关联创建出的订阅记录（用于幂等与追溯）
- `user_subscriptions`：用户订阅（start/end；支持同一用户并发多条记录；当前生效订阅定义为 `start_at <= now < end_at`；扣费按 end_at 最早到期优先）

> 不记录到审计/日志：用户输入内容、模型输出全文、任何明文数据面 Token/Web Session。
