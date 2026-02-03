# realms

## 目的
规划并实现 **Realms**：一个 **OpenAI 风格 API 中转服务**（含 Codex/OpenAI 上游接入，对外提供 OpenAI 风格接口），并沉淀与 Codex CLI 协议、鉴权链路、用量/口径相关的可验证结论。

## 模块概述
- **职责:** 中转服务方案整合（高可用/流式/SSE）、鉴权与凭据管理研究、可落地实现路径规划
- **状态:** 🚧可用（MVP）
- **最后更新:** 2026-01-28

## 关键入口
- 代码入口（实现）：
  - `cmd/realms/main.go`
  - `internal/server/app.go`
- `internal/api/openai/handler.go`
- `internal/scheduler/`
- `internal/upstream/`

## 运行行为
- 主 HTTP 监听启动失败会直接退出进程（返回非零），避免进程空跑等待信号。

## 配置要点（Realms）

### 上游限额（Account Limits）

- 管理入口：
  - OpenAI API key：管理后台 `GET /admin/channels` → 渠道「设置」弹窗 → “密钥”
  - Codex OAuth 账号：管理后台 `GET /admin/channels` → 渠道「设置」弹窗 → “授权账号”
- 说明：渠道/密钥/账号的 `Sessions/RPM/TPM` 限额能力已移除；下述字段口径仅作历史记录。
- 字段与口径（历史，不再生效）：
  - `sessions`：会话 ID 级别的 sessions 上限（基于 `user_id + route_key_hash` 的粘性绑定计数；当请求缺少 route key 时不生效）
  - `rpm`：每分钟请求上限（滑动窗口计数）
  - `tpm`：每分钟 tokens 上限（input+output 总和；依赖上游返回 usage，缺失 usage 的请求不会计入）

### 模型价格表导入

- 配置入口：管理后台 `GET /admin/models` → “导入价格表”（上传/粘贴 JSON）。
- 支持格式：Realms 简化格式（`*_usd_per_1m`）与 LiteLLM 常见字段（`*_cost_per_token` → 自动换算为 `usd_per_1m`）。
- 写入策略：
  - 新模型：默认以 `status=0`（禁用）创建，避免“导入即对外可用”的风险。
  - 已存在模型：仅更新价格字段（不改 `public_id/status/owned_by`）。

### 模型库字段填充（OpenRouter）

用于减少手填定价与归属方的出错概率。

- 入口：管理后台 `GET /admin/models` → “新增模型”弹窗 → 点击「从模型库填充」。
- 交互：
  - 输入 `public_id`（对外模型 ID）时支持下拉提示，可按关键字检索并选择候选 `model_id`。
  - 一般情况下不需要写前缀；如遇同名冲突可用 `provider/model_id`（例如 `openai/gpt-4o`）明确指定。
  - 点击「从模型库填充」后：按 `model_id` 远程查询并自动填充：
  - `owned_by`（用于展示与图标映射）
  - `input_usd_per_1m/output_usd_per_1m/cache_input_usd_per_1m/cache_output_usd_per_1m`（单位：USD / 1M Token）
- 数据源：OpenRouter（`https://openrouter.ai/api/v1/models`）。
- 注意：查询只填充表单，不会自动保存；仍需管理员确认后点击“保存”创建模型。
- 备注：OpenRouter 的定价字段为 `USD/token`（`pricing.prompt/pricing.completion`）；Realms 会自动换算为 `USD/1M Token`。若返回 `input_cache_read/input_cache_write`，会据此填充缓存单价；若缺失则缓存单价填 `0`（可后续手动调整或导入价格表覆盖）。

### 运维与排障

- 健康检查：`GET /healthz`（返回 env/version/date、DB 状态等；构建版本 release 可通过 `-ldflags -X` 注入）
- Dev 调试落盘（默认关闭）：
  - 配置：环境变量（`.env`）`REALMS_DEBUG_PROXY_LOG_ENABLE/REALMS_DEBUG_PROXY_LOG_DIR`
  - 仅在 `env=dev` 且 `REALMS_DEBUG_PROXY_LOG_ENABLE=true` 时生效
  - 强制脱敏：仅记录请求元信息，不落盘用户输入内容/密钥

### Docker 部署（docker compose）

- Compose 文件：仓库根目录 `docker-compose.yml`（`mysql + realms`）
- Realms 镜像：默认从 Docker Hub 拉取 `flowerrealm/realms`（可在 `.env` 中用 `REALMS_IMAGE=...` 覆盖/固定 tag/切到本地构建镜像）
- 启动/升级：`docker compose pull realms && docker compose up -d`
- 端口映射：默认仅本机 `127.0.0.1:18080->8080`（可在 `.env` 中用 `REALMS_HTTP_PORT=...` 覆盖宿主端口）
- Realms 配置方式：不再支持/不再依赖 `config.yaml`；启动时会自动加载当前目录的 `.env`（若存在），配置仅来自环境变量（如 `REALMS_DB_DSN`）
- MySQL 数据持久化：命名卷 `mysql_data` 挂载到 `/var/lib/mysql`
- MySQL 端口暴露：默认 `0.0.0.0:${MYSQL_HOST_PORT:-3306}->3306`（可用 `MYSQL_HOST_PORT/MYSQL_BIND_IP` 调整；公网部署务必配合防火墙/最小权限账号）
- 纯 HTTP 场景：如需 Web 登录功能，需允许非 Secure Cookie（`REALMS_DISABLE_SECURE_COOKIES=true`）；公网部署建议加反向代理/TLS（本方案不包含）

### 开发（make dev）

- `make dev` 仅启动本地（正常模式）：`http://127.0.0.1:8080/`（air 热重载）
- `make dev` 不会自动启动 Docker / MySQL；如需使用 docker compose 请自行启动（例如：`docker compose up -d mysql`）

### 反向代理 / 外部访问地址
- 站点地址（对外基础地址）优先级：
  1. `app_settings.site_base_url`（管理后台「系统设置」可配置）
  2. `app_settings_defaults.site_base_url`（启动期默认值；仅当数据库未覆盖该键时生效）
  3. `server.public_base_url`
  4. 按请求推断（仅在启用且请求来自 `trusted_proxy_cidrs` 时读取 `X-Forwarded-*`；否则回退使用 `Host/TLS` 推断）
  用途：页面展示、支付回调/返回地址与 Codex OAuth 回跳链接生成。
- `security.trust_proxy_headers` / `security.trusted_proxy_cidrs`：控制是否信任 `X-Forwarded-*` 头（默认不信任；启用后仅信任来自 CIDR 白名单的请求；如需信任所有来源可显式配置 `0.0.0.0/0` 与 `::/0`，不推荐）。

### 管理后台配置入口（UI）

- **运行期配置（数据库）**：SPA 页面 `/admin/settings`（仅 root）；对应 API：`GET/PUT /api/admin/settings`
  - 写入表：`app_settings`
  - 优先级：高于启动期默认值（`app_settings_defaults`）
  - 特点：保存后无需重启即可生效（适用于少量运行态配置与功能开关）
- **启动配置（环境变量/.env）**：通过部署侧维护 `.env`/环境变量（修改后需重启服务；管理后台不提供写回入口）
- **侧边栏导航**：当入口较多或窗口高度较小时，左侧导航列表区域会自动滚动，避免菜单溢出导致底部入口不可见。

## Web/管理后台：站点图标（favicon/Logo）

用于统一站点品牌呈现（浏览器标签页图标 + 页面导航区域 Logo）。

- 图标源文件：`internal/assets/realms_icon.svg`（内置到二进制）
- 资源路由：
  - `GET /assets/realms_icon.svg`
  - `GET /favicon.ico` → 永久重定向到 `/assets/realms_icon.svg`
- 前端引用：`web/index.html`（`<link rel="icon" ...>`）与 SPA 组件。

### 渠道健康检查（测试连接）

- 入口：管理后台「上游渠道」页的“测试”按钮（SPA）。
- API（root 会话）：
  - `GET /api/channel/test/:channel_id`：测试单个渠道
  - `GET /api/channel/test`：测试所有渠道
- Codex OAuth：仅通过 `/v1/responses` 请求上游；不再自动兜底重试旧版 `/responses` 路径。

### CORS（浏览器跨域调用）
- 当前服务未提供可配置的内置 CORS；如需浏览器跨域直连 `/v1/*` 或 `/api/*`，建议在反向代理层添加 CORS Header 或改为同域部署。

### OAuth Apps（外部客户端授权）

- 管理入口：管理后台 `GET /admin/oauth-apps`（创建应用、配置 `redirect_uri` 白名单、生成/轮换 `client_secret`）。
- 授权流程（Authorization Code → Realms API Token）：
  1. 外部应用跳转：`GET /oauth/authorize?response_type=code&client_id=...&redirect_uri=...&state=...&scope=...`
  2. 浏览器打开授权页（SPA：`/oauth/authorize`）；如未登录先在 `/login` 登录，再回到授权页完成确认（页面内部调用 `/api/oauth/authorize`）
  3. 授权成功后跳转回外部应用：`redirect_uri?code=...&state=...`
  4. 外部应用交换 token：`POST /oauth/token` → 返回 `access_token=rlm_...`（可直接调用 `/v1/*`）
- 安全边界：`redirect_uri` 必须与白名单精确匹配；`state` 必填；授权码短期有效且一次性消费。
- token 归属：OAuth 签发的 token 落库在 `user_tokens`（名称 `oauth:<client_id>`），用户可在 Web 控制台 `/tokens` 撤销。

### 自用模式（self_mode）

- 配置开关：`self_mode.enable=true`（或环境变量 `REALMS_SELF_MODE_ENABLE=true`）
- 路由与 UI 裁剪（硬禁用：相关 API 不注册，返回 404；前端入口也会隐藏/不可用）：
  - 计费/支付：`/api/billing/*`、`/api/pay/*`、`/api/webhooks/subscription-orders/*`、`/api/admin/subscriptions|orders|payment-channels`
  - 工单：`/api/tickets*`、`/api/admin/tickets*`
- 数据面配额策略：自用模式下不再要求订阅/余额；仍会记录 `usage_events`（用于用量统计与排障）。
- 后台任务：自用模式下不会启动工单附件清理 loop（避免无意义的定时扫描）。

### 功能禁用（Feature Bans）

- 配置入口：管理后台页面 `/admin/settings` → 「功能禁用」（对应 API：`GET/PUT /api/admin/settings`）
- 存储：`app_settings.feature_disable_*`（bool；`true` 表示禁用；缺省时回退 `app_settings_defaults.feature_disable_*`；仍缺省则视为启用）
- 生效方式：
  - **隐藏 UI 入口**（Web/管理后台侧边栏等）
  - **后端拒绝访问**：命中的路由会直接返回 **404**（包含 root；与 `self_mode` “未注册即 404” 风格一致）
- 安全护栏：系统设置页（`/admin/settings`）不会被禁用，避免把自己锁在外面。
- 与 `self_mode` 的关系：
  - `self_mode` 仍是“硬禁用”（启动时不注册路由）
  - `feature_disable_*` 是“运行态禁用”（路由存在，但被 gate 拦截返回 404）
  - `self_mode` 会强制禁用计费与工单（即使 feature key 未设置）
- 救援方式：
  - 首选：进入 `/admin/settings` 重新启用（取消勾选后保存，或点击「恢复为配置文件默认」）。
  - 兜底：直接从数据库 `app_settings` 表删除 `feature_disable_*` 对应记录。

### 数据面语义（由 Feature Bans 推导）

`feature_disable_*` 除了用于“隐藏入口 + 路由 404”外，部分开关会直接影响数据面语义（“禁用=语义切换”）：

- `feature_disable_billing=true`：数据面进入 free mode
  - 不校验订阅/余额（不会返回 `订阅未激活/余额不足`）
  - 仍记录 `usage_events`（用于用量统计与排障）
- `feature_disable_models=true`：数据面进入模型穿透（model passthrough）
  - 关闭 `/models`、`/admin/models*`、`/v1/models`
  - 不要求模型已启用（跳过模型白名单）
  - 不要求模型存在可用渠道绑定（跳过 `channel_models` 白名单）
  - `model` 直接透传到上游（不做 alias rewrite）
  - **注意：**非 free mode 下仍要求模型定价存在（`managed_models` 有记录），用于配额预留与计费口径；free mode 下可允许任意 model

### 渠道组树形路由（Channel Group Routing）

- 数据面入口：`internal/api/openai/handler.go` 使用 `internal/scheduler/group_router.go`，从 **`channel_groups.name='default'` 根组**开始选择叶子渠道并执行 failover。
- 路由编排 SSOT：`channel_group_members`（父组 → 子组/渠道）。
  - **默认模式（无指针）**：候选叶子渠道排序按 `probe_pending` → `promotion` → `priority` → `fail_score` → `channel_id`（稳定排序）。
  - **指针模式（有指针）**：将整棵树按稳定 DFS 展开为 **Channel Ring**（叶子渠道序列），并从指针位置开始遍历一圈（到底从头再来）。
- 约束：
  - 模型绑定白名单：仅允许命中 `channel_models` 的渠道（`cons.AllowChannelIDs`）。
  - 分组过滤：按用户分组集合（`cons.AllowGroups`）筛选可用叶子渠道。
- 自动 ban：
  - `internal/scheduler/state.go` 维护 `channel_ban_until + channel_ban_streak`；
  - 连续可重试失败达到阈值后进入 ban，并在调度选择时直接跳过；成功会清零。
- 渠道指针（运行态）：
  - 管理后台 `GET /admin/channels` 可一键将某个渠道设置为 **渠道指针**（内存态，不落库），作为“应该使用什么渠道”的**唯一标定（SSOT）**：
    - 指针开启时：数据面会从指针渠道开始尝试；若该渠道不可用则按 ring 顺序继续向后，直到遍历一圈（到底从头再来）。
    - 若指针指向的渠道不在当前 ring（例如不在 `default` 树，或树结构变更后被移除）：运行时会将该渠道追加到 ring（尾部）以确保“设为指针”可以立即生效；不可用时仍会按 ring 继续 failover。
    - 当指针指向的渠道进入 ban 时：指针会自动轮转到 ring 的下一个渠道（自动跳过仍处于 ban 的渠道）。
    - 指针会 **覆盖会话粘性绑定/亲和**（避免“设置了但仍走旧渠道”）。
  - 指针设置入口：`POST /api/channel/{channel_id}/promote`（root 会话；语义为“设为指针”（设为渠道指针），并清除该渠道封禁）。

### 管理后台：分组树

- 列表页：`GET /admin/channel-groups`（仅 root），每行提供“进入”按钮跳转到组详情。
- 组详情页：`GET /admin/channel-groups/{group_id}`（仅 root）
  - 支持新建子组、添加渠道到该组、移除成员、拖拽排序（`POST /admin/channel-groups/{group_id}/children/reorder`）。

## Web 控制台：模型图标库

用于在 SPA 控制台与管理后台的模型列表中展示“模型供应商/品牌”图标，提升可读性。

- 图标来源：`@lobehub/icons-static-svg`（MIT），通过 jsDelivr CDN 引用（无需前端构建链路）
- 映射逻辑：`internal/icons/model_icons.go`
  - 优先使用数据库字段 `owned_by`（展示用 owner）
  - 若 `owned_by` 为空，则回退用 `model_id`（public_id）关键词匹配
- 输出方式：后端在模型相关 API 响应中附带 `icon_url`（例如 `GET /api/user/models/detail`、`GET /api/models/`）。

## Web 控制台：账号体系（邮箱/账号名/密码）

- 登录支持“邮箱或账号名（username）+ 密码”；`username` 为必填字段（注册必须设置），且**不可修改**、**唯一**、**区分大小写**。
- 账号名规则：仅允许字母/数字（禁止空格与特殊字符）。
- 账号设置页：`/account`
  - 账号名只读展示（不可修改）
  - 修改邮箱（强制验证码校验）
  - 修改密码（普通用户需旧密码）
  - 任一变更成功后强制登出（清理该用户所有 session）
- 管理后台用户管理：`/admin/users`
  - 展示账号名（不可编辑）
  - 修改邮箱不需要验证码（root 直接修改）
  - 重置密码后强制登出目标用户
  - 配置用户分组：`user_groups`（多选；强制包含 `default`；用于上游调度筛选渠道与订阅购买权限）

## Web 控制台：工单系统（工单 + 消息线程 + 附件）

- 用户入口：`/tickets`
  - 创建工单：`GET /tickets/new` + `POST /tickets/new`
  - 查看工单：`GET /tickets`（仅本人） / `GET /tickets/{ticket_id}`（仅本人）
  - 追加回复：`POST /tickets/{ticket_id}/reply`（仅本人；工单关闭后禁止回复）
  - 下载附件：`GET /tickets/{ticket_id}/attachments/{attachment_id}`（仅本人）
- 管理入口（仅 `root`）：`/admin/tickets`
  - 列表/详情：`GET /admin/tickets` / `GET /admin/tickets/{ticket_id}`
  - 回复：`POST /admin/tickets/{ticket_id}/reply`
  - 关闭/恢复：`POST /admin/tickets/{ticket_id}/close` / `POST /admin/tickets/{ticket_id}/reopen`
  - 下载附件：`GET /admin/tickets/{ticket_id}/attachments/{attachment_id}`
- 附件存储与限制：
  - 本地目录：`tickets.attachments_dir`（建议容器部署时挂载 volume 持久化）
  - 过期时间：`tickets.attachment_ttl`（默认 7 天；到期后后台定时清理）
  - 上传限制：`tickets.max_upload_bytes`（单次上传附件总大小上限；服务端会额外预留少量 multipart 开销）
- 安全要点：
  - 上传路由在 `CSRF` 之前应用 `MaxBytesReader`，避免 multipart 在解析时先读取超大请求体
  - 附件下载严格鉴权（用户仅能下载自己的；管理员仅 root）
  - 附件路径完全由服务端生成并校验，避免路径穿越

## Web 控制台：公告（管理员发布 / 用户只读 / 未读数量提示）

- 用户入口：
  - 列表：`GET /announcements`
  - 详情：`GET /announcements/{announcement_id}`（进入即标记已读）
- 未读提示：
  - 登录进入控制台 `GET /dashboard` 时，如果存在未读公告，页面会展示未读数量提示；用户可在“公告”页查看，进入详情会标记已读
- 管理入口（仅 `root`）：
  - `GET /admin/announcements`（创建/发布/撤回/删除公告）

## Web 控制台：支付与按量计费（充值/订阅）

- 用户入口（SPA）：
  - 余额充值：`GET /topup`
  - 支付页：`GET /pay/{kind}/{order_id}`（`kind`：`subscription` / `topup`）
- Web API（Cookie Session）：
  - `GET /api/billing/subscription` / `POST /api/billing/subscription/purchase`
  - `GET /api/billing/topup` / `POST /api/billing/topup/create`
  - `GET /api/billing/pay/{kind}/{order_id}` / `POST /api/billing/pay/{kind}/{order_id}/start` / `POST /api/billing/pay/{kind}/{order_id}/cancel`
- 支付回调（无需登录）：
  - Stripe：`POST /api/pay/stripe/webhook/{payment_channel_id}`（按渠道验签 + 幂等）
  - EPay：`GET /api/pay/epay/notify/{payment_channel_id}`（按渠道验签 + 幂等，返回 `success`/`fail`）
- 生效规则：
  - 充值订单（topup）：支付成功后增加 `user_balances.usd`，用于按量计费
  - 订阅订单（subscription）：支付成功后创建/激活 `user_subscriptions`，并更新订单状态为“已生效”（保留订单记录便于追溯）
- 配置来源：
  - 支付渠道：管理后台页面 `/admin/payment-channels`（表：`payment_channels`）
  - 计费开关与充值比例：管理后台页面 `/admin/settings`（表：`app_settings`）+ 环境变量默认值
  - 套餐：管理后台页面 `/admin/subscriptions`（表：`subscription_plans`）
- 说明：订单关闭（cancel）后若仍完成支付，服务端不会自动入账/生效，需要人工退款处理。

## Web/管理后台：提示消息

- SPA 统一用组件状态展示提示，不再依赖 SSR 的 `?msg/?err` 参数与 PRG/AJAX 表单方案。

## 用户可见文案（中文化）

- Web 控制台与管理后台的固定文案统一为中文（保留 `Realms`/`Codex`/`OpenAI`/`OAuth` 等专有名词与技术标识符）。
- 术语口径（与 UI/README 保持一致）：
  - “Token（凭证/密钥）”统一为“令牌”
  - “Token（计量单位）”保留为 Token（如“每 1M Token”）
  - Channel/Endpoint/Credential 统一对应“渠道/端点/凭证”

## Codex OAuth（Realms）

### 入口
- 管理后台：`/admin` → `codex_oauth` 渠道（自动创建）→ 渠道端点页 `#accounts` → 账号列表右上角 `+`（弹窗内提供“快捷授权/手工录入”）

### 授权方式
- **粘贴回调 URL（默认）**：在账号列表右上角点击 `+` → “快捷授权” → “发起授权（新窗口打开）”，浏览器完成登录后会回调到 `http://localhost:{服务端口}/auth/callback`（固定 localhost，用于模拟 codex 登录）。若回调页无法访问/无法被服务接收，复制地址栏中的完整回调 URL（包含 `code/state`）粘贴到“完成授权”表单完成入库。
- **state 有效期**：服务端会短期缓存 `state → code_verifier/endpoint/actor`（DB：`codex_oauth_pending`），默认有效期约 30 分钟；超时或已被消费会提示“state 无效或已过期”，重新发起授权即可。
- **token 换取超时排查**：token 换取在服务端发生（`POST https://auth.openai.com/oauth/token`），需确保运行 Realms 的机器可访问该地址；如遇 `TLS handshake timeout`/`i/o timeout` 可配置代理（`HTTPS_PROXY/HTTP_PROXY`）或排查网络/DNS。
- **回调后管理页自动刷新（best-effort）**：当回调页可被服务接收时，回调窗口会尝试通知原管理页刷新账号列表；若浏览器因 COOP 策略清空 `window.opener`，则回调窗口会跳回管理后台并通过 `localStorage` 广播刷新（原管理页自动更新）。

### claims 解析（对齐 CLIProxyAPI）
- `account_id` 来源：`id_token` 的 `https://api.openai.com/auth.chatgpt_account_id`（并保留少量兜底字段以兼容差异）
- 订阅状态展示：`https://api.openai.com/auth.chatgpt_plan_type` 与 `chatgpt_subscription_active_start/until`
- 管理后台账号列表会展示“订阅有效期”进度条，并在有效期内展示到期时间与剩余天数。

### Channel 健康测试（admin）
- 管理后台 `Channels` 页的“测试”会对该渠道发起一个轻量的**流式（SSE）**请求：`openai_compatible/codex_oauth` 走 `/v1/responses`，`anthropic` 走 `/v1/messages`。
- 无模型绑定时会回退到默认模型：`openai_compatible/codex_oauth` 使用 `gpt-5.2`，`anthropic` 使用 `claude-3-5-sonnet-latest`；并显式 `stream=true`（避免部分上游对非流式返回 400）。

### Channel 用量统计（admin）
- 管理后台 `Channels` 页支持按区间展示渠道用量统计（默认今天；按管理后台时区解析）：总消耗（USD）、总 Token（输入+输出，含缓存 Token）与缓存命中率（`(cached_input_tokens+cached_output_tokens)/(input_tokens+output_tokens)`）；查询参数为 `start/end`（格式：`YYYY-MM-DD`）。

### 用量统计：请求级明细（user/admin）
- 请求级明细页（用户 `/usage`、管理员 `/admin/usage`）按“每一次请求”展示，包含请求/响应、状态码、耗时、错误信息等字段。
- 控制区（start/end/limit）已放入请求明细列表内部，避免页面顶部单独控制区（交互位置对齐 new-api）。
- 快捷区间按钮（今天/昨天/7天/30天）切换后会自动提交筛选表单并刷新数据，避免“日期变了但数据没变”的误解（Web 端目前提供今天/昨天/7天）。
- 分页使用 keyset（`before_id`/`after_id`），在大数据量下比 offset 更稳定。

- 用户控制台 `/usage`：在汇总卡片下展示“请求明细”表（按每次请求记录 `request_id`、接口、状态码、耗时、输入/输出/缓存 Token、费用、渠道、错误等），支持 `start/end` 区间与分页。
- 管理后台 `/admin/usage`：新增同口径的“请求明细”表（全站视角，额外展示用户信息），用于排查单次请求的结果与计费/用量口径；时间展示与 `start/end` 解析按管理后台时区（系统设置 `admin_time_zone`；默认 `Asia/Shanghai`，可通过 `app_settings_defaults.admin_time_zone` 调整）。
- 数据来源：`usage_events` 新增字段 `endpoint/status_code/latency_ms/error_class/error_message/is_stream/request_bytes/response_bytes`（仅元数据，不记录任何用户输入内容或模型输出全文）。
- API：`GET /api/usage/events` 同步返回上述字段；可选 `start/end`（YYYY-MM-DD，UTC）按区间过滤。
- 备注：流式（SSE）请求会 best-effort 从 SSE 的 `data:` JSON 事件里提取 `usage`（含 `*_tokens_details.cached_tokens`）用于结算与请求明细展示；若上游未返回 usage，则 `input_tokens/output_tokens` 仍可能为空，并以 reserved 兜底结算（仍会记录状态码/耗时与 `is_stream`）。
- 断联分类：流式请求中断会落到 `error_class`（例如 `client_disconnect/stream_idle_timeout/stream_event_too_large/stream_read_error/stream_max_duration`），用于区分“客户端断开”与“上游/读取异常”。

### Codex 上游请求
- `codex_oauth` 渠道仅透传 `/v1/responses` 请求给上游（不再支持 legacy `/responses` 兼容改写/自动降级）。
- 服务会注入 OAuth access token（`Authorization: Bearer ...`）并补齐少量 Codex 风格 Header：`Accept: text/event-stream`、`Connection: Keep-Alive`、`Session_id`、`Version`、`User-Agent`、`Openai-Beta: responses=experimental`、`Originator: codex_cli_rs`、`Chatgpt-Account-Id`。

### 账号额度/限额（后台自动刷新）
- 管理后台账号列表会展示 Codex usage 的 **credits** 与 **rate_limit.primary/secondary 两个窗口**（`used_percent/reset_at`），并回显刷新错误信息。
- 服务端后台每 **10 分钟**刷新一次所有 Codex OAuth 账号额度，并将结果落库到 `codex_oauth_accounts.quota_*` 字段。
- UI 口径映射（团队账号 Team）：
  - `primary_window` → **5 小时额度**（$6 / 5h）
  - `secondary_window` → **周限额与代码审查额度**（$20 / week）
- 管理后台会按 `used_percent` **估算**窗口剩余金额（USD）：`remaining = cap * (100 - used_percent) / 100`，并同时展示重置时间 `reset_at`。

## Anthropic Messages（Realms）

### 入口
- 数据面：`POST /v1/messages`（Anthropic Messages 兼容；用于 Claude Code / Anthropic SDK）
- 管理后台：`/admin` → `anthropic` 渠道 → 渠道配置（端点/基础地址 + Keys 同页管理）

### 中转策略（直通 + 最少改写）
- Realms 不做 OpenAI↔Anthropic 协议转换；`/v1/messages` 仅调度到 `anthropic` 类型渠道，并原样转发请求体（仅在缺省时补 `max_tokens`）。
- 上游鉴权：注入 `x-api-key`；并默认补 `anthropic-version: 2023-06-01`（下游显式设置同名 header 时不会覆盖）。

### 模型与绑定
- 与 `/v1/responses` 一致：默认走 `managed_models + channel_models` 白名单与 alias 重写；在 `feature_disable_models=true` 时进入模型穿透（`model` 直接透传到上游）。

## OpenAI Compatible（Realms）

### 入口
- 管理后台：`/admin` → `openai_compatible` 渠道 → 渠道配置（端点/基础地址 + Keys 同页管理）

### 分组（参考 new-api 思路，但不是租户）
- 用户分组：`user_groups`（默认包含 `default`；用户可加入多个组；管理后台用户资料可配置）
- 渠道分组：`upstream_channels.groups`（默认 `default`；逗号分隔多个分组；管理后台 Channels/Endpoints 可配置）
- 分组字典管理：`/admin/channel-groups`（新增/禁用/删除）；删除为强制删除：会移除用户/渠道对该分组的引用；若某渠道仅属于该分组，删除时会自动禁用该渠道并回退到 `default`
- 数据面调度：`/v1/*` 请求会按用户分组筛选可用渠道，failover/粘性绑定不会绕过该约束

### 数据面：粘性路由与缓存口径

- 请求体缓存：`internal/middleware/body_cache.go` 会把 body 缓存在 context 中，使 handler 可在“解析/校验 → 转发 → 重试”场景下重复读取。
- RouteKey（用于 prompt caching 粘性）提取顺序：
  1. JSON body 顶层字段 `prompt_cache_key`
  2. header 兜底：`Prompt-Cache-Key` / `X-Prompt-Cache-Key` / `X-RC-Route-Key` / `Conversation_id` / `Session_id` / `Idempotency-Key`（含常见大小写变体）
  - routeKey 仅用于 hash（不落库/不打日志），并限制最大长度以避免异常输入拖慢请求。
- 粘性绑定（scheduler）：以 `user_id + routeKeyHash` 做短期绑定（默认 30 分钟），命中后会续期；当绑定不满足约束（分组/渠道限制）或凭证处于冷却时会自动忽略并选择新的可用上游。
- 冷却策略（scheduler）：当上游返回可重试状态码时会触发 failover，并对凭证施加短期冷却（默认 30s）。可重试状态码包含 `401/402/403/408/429/502/503/504` 与其他 `5xx`；当上游状态码为 `429` 时冷却时间会更长（默认 60s，即 2×基准冷却）。
- 缓存 token 统计：从上游响应 `usage` 中提取 `cached_input_tokens/cached_output_tokens`（兼容 `*_tokens_details.cached_tokens`）用于用量页的缓存命中率与成本口径展示。
- 缓存计费口径（按模型定价字段拆分；并对缓存 tokens 做子集裁剪）：
  - `cached_input_tokens = min(cached_input_tokens, input_tokens)`；`cached_output_tokens = min(cached_output_tokens, output_tokens)`
  - 成本（USD）= 非缓存输入×`input_usd_per_1m` + 非缓存输出×`output_usd_per_1m` + 缓存输入×`cache_input_usd_per_1m` + 缓存输出×`cache_output_usd_per_1m`（均按 /1M Token 换算；最终截断到 6 位小数，对齐 DB `DECIMAL(20,6)`）
- SSE 转发（无超时/大小限制）：
  - `internal/upstream/PumpSSE` 默认不设置 idle-timeout / max duration / 单行长度限制（避免误断联与误判超长事件）。

## 开发热重载（自动重启）
- 推荐使用 `air`：监听 Go/模板（embed HTML）/迁移（embed SQL）/配置变更，自动重新编译并重启进程。
- 启动方式：`make dev`（会安装 `air` 到 `.tmp/bin`，并通过 `scripts/dev.sh` 生成本地 `.env`）。

- 已执行方案包：
  - `helloagents/history/2026-01/202601161351_strip_msg_query/`
    - [task.md](../../history/2026-01/202601161351_strip_msg_query/task.md)
  - `helloagents/history/2026-01/202601160555_group_multi_membership/`
    - [why.md](../../history/2026-01/202601160555_group_multi_membership/why.md)
    - [how.md](../../history/2026-01/202601160555_group_multi_membership/how.md)
    - [task.md](../../history/2026-01/202601160555_group_multi_membership/task.md)
  - `helloagents/history/2026-01/202601152042_channel_grouping/`
    - [why.md](../../history/2026-01/202601152042_channel_grouping/why.md)
    - [how.md](../../history/2026-01/202601152042_channel_grouping/how.md)
    - [task.md](../../history/2026-01/202601152042_channel_grouping/task.md)
  - `helloagents/history/2026-01/202601152017_user_account_management/`
    - [why.md](../../history/2026-01/202601152017_user_account_management/why.md)
    - [how.md](../../history/2026-01/202601152017_user_account_management/how.md)
    - [task.md](../../history/2026-01/202601152017_user_account_management/task.md)
  - `helloagents/history/2026-01/202601141705_channel_test_stream/`
    - [task.md](../../history/2026-01/202601141705_channel_test_stream/task.md)
  - `helloagents/history/2026-01/202601141649_channel_test_dialog/`
    - [task.md](../../history/2026-01/202601141649_channel_test_dialog/task.md)
  - `helloagents/history/2026-01/202601141640_default_allow_private_baseurl/`
    - [task.md](../../history/2026-01/202601141640_default_allow_private_baseurl/task.md)
  - `helloagents/history/2026-01/202601141630_single_endpoint_per_channel/`
    - [why.md](../../history/2026-01/202601141630_single_endpoint_per_channel/why.md)
    - [how.md](../../history/2026-01/202601141630_single_endpoint_per_channel/how.md)
    - [task.md](../../history/2026-01/202601141630_single_endpoint_per_channel/task.md)
  - `helloagents/history/2026-01/202601141611_rebrand_realms/`
    - [why.md](../../history/2026-01/202601141611_rebrand_realms/why.md)
    - [how.md](../../history/2026-01/202601141611_rebrand_realms/how.md)
    - [task.md](../../history/2026-01/202601141611_rebrand_realms/task.md)
  - `helloagents/history/2026-01/202601141531_user_ban_session/`
    - [task.md](../../history/2026-01/202601141531_user_ban_session/task.md)
  - `helloagents/history/2026-01/202601141449_channel_test/`
    - [why.md](../../history/2026-01/202601141449_channel_test/why.md)
    - [how.md](../../history/2026-01/202601141449_channel_test/how.md)
    - [task.md](../../history/2026-01/202601141449_channel_test/task.md)
  - `helloagents/history/2026-01/202601141423_remove_group/`
    - [why.md](../../history/2026-01/202601141423_remove_group/why.md)
    - [how.md](../../history/2026-01/202601141423_remove_group/how.md)
    - [task.md](../../history/2026-01/202601141423_remove_group/task.md)
  - `helloagents/history/2026-01/202601141419_makefile_dev/`
    - [task.md](../../history/2026-01/202601141419_makefile_dev/task.md)
  - `helloagents/history/2026-01/202601141411_dev_hot_reload/`
    - [task.md](../../history/2026-01/202601141411_dev_hot_reload/task.md)
  - `helloagents/history/2026-01/202601131914_codex/`
    - [why.md](../../history/2026-01/202601131914_codex/why.md)
    - [how.md](../../history/2026-01/202601131914_codex/how.md)
    - [task.md](../../history/2026-01/202601131914_codex/task.md)
  - `helloagents/history/2026-01/202601141350_upstream_delete/`
    - [why.md](../../history/2026-01/202601141350_upstream_delete/why.md)
    - [how.md](../../history/2026-01/202601141350_upstream_delete/how.md)
    - [task.md](../../history/2026-01/202601141350_upstream_delete/task.md)
  - `helloagents/history/2026-01/202601141319_subscription_purchase/`
    - [why.md](../../history/2026-01/202601141319_subscription_purchase/why.md)
    - [how.md](../../history/2026-01/202601141319_subscription_purchase/how.md)
    - [task.md](../../history/2026-01/202601141319_subscription_purchase/task.md)
  - `helloagents/history/2026-01/202601141229_oauth_upstreams/`
    - [why.md](../../history/2026-01/202601141229_oauth_upstreams/why.md)
    - [how.md](../../history/2026-01/202601141229_oauth_upstreams/how.md)
    - [task.md](../../history/2026-01/202601141229_oauth_upstreams/task.md)
  - `helloagents/history/2026-01/202601140645_ui-console-admin/`
    - [why.md](../../history/2026-01/202601140645_ui-console-admin/why.md)
    - [how.md](../../history/2026-01/202601140645_ui-console-admin/how.md)
    - [task.md](../../history/2026-01/202601140645_ui-console-admin/task.md)
  - `helloagents/history/2026-01/202601140558_mysql_autocreate_db/`
    - [task.md](../../history/2026-01/202601140558_mysql_autocreate_db/task.md)
  - `helloagents/history/2026-01/202601140614_mysql_wait_ready/`
    - [task.md](../../history/2026-01/202601140614_mysql_wait_ready/task.md)
  - `helloagents/history/2026-01/202601140620_mysql_migrations_multistmt/`
    - [task.md](../../history/2026-01/202601140620_mysql_migrations_multistmt/task.md)
  - `helloagents/history/2026-01/202601140625_template_content_render/`
    - [task.md](../../history/2026-01/202601140625_template_content_render/task.md)
- 变更历史索引：[helloagents/history/index.md](../../history/index.md)
- 调研文档：
  - [Codex CLI wire API](../research/codex_cli_wire_protocol.md)
  - [claude-proxy 路由与 failover 机制](../research/claude-proxy-routing.md)
  - [new-api 端口通信与转发实现](../research/new-api_api_port_communication.md)

## 变更历史
- 202601161926_payment_channels - 支付渠道化：新增 `payment_channels`（按渠道独立配置），管理后台 `/admin/payment-channels`，支付页按渠道选择，并新增按渠道回调路由（Stripe/EPay）
- 202601161610_payments - 支付与按量计费：新增充值与支付页（`/topup`、`/pay/{kind}/{order_id}`），接入 EPay/Stripe 回调入账/生效（验签 + 幂等）
- 202601161558_order_review_cleanup - 订单审批去重：移除“标记已支付并生效”，新增“不批准”，并统一通过更新订单状态完成处理（保留订单记录）
- 202601161525_subscription_orders - 订阅订单：购买先创建订单（待支付），支付后自动生效；管理员可手动批准生效（新增 `/admin/orders`）
- 202601152105_upstream_groups_keyword - 修复 MySQL 8 `GROUPS` 保留字导致的上游渠道查询失败：SQL 引用 `upstream_channels.groups` 时使用反引号包裹
- 202601152116_remove_localhost_note - 清理 base_url 地址范围限制相关多余字段/文案
- 202601152055_always_allow_private_baseurl - base_url 校验策略调整：移除禁用逻辑与相关开关/文案
- 202601152042_channel_grouping - 引入渠道分组（非租户）：用户分组演进为 `user_groups`（强制 default，多选），渠道分组为 `upstream_channels.groups`；调度器按分组筛选；管理后台支持配置
- 202601141705_channel_test_stream - 渠道测试改为流式（SSE）并展示 TTFT/示例输出
- 202601141649_channel_test_dialog - 渠道测试增强：对话式输入 + 展示示例输出
- 202601141640_default_allow_private_baseurl - base_url 校验策略调整（移除地址范围限制相关开关）
- 202601141630_single_endpoint_per_channel - 上游渠道收敛为单 Endpoint（Codex OAuth 多账号 / openai_compatible 多 Key）
- 202601141611_rebrand_realms - 品牌改名：Realms（入口/构建产物/环境变量/控制台文案统一；不保留旧命名兼容）
- 202601141531_user_ban_session - 修复封禁/禁用用户后已登录 Web Session 仍可继续访问（强制登出）
- 202601141449_channel_test - 管理后台渠道健康测试（延迟/可用性）与最近一次结果展示
- 202601141423_remove_group - 移除 group/租户概念（单租户化；上游/定价全局；用户用量查询 API）
- 202601141350_upstream_delete - 管理后台上游硬删除能力（channel/endpoint/credential/account）
- 202601141319_subscription_purchase - 订阅购买与额度限制（¥12/月；5h/7d/30d 滚动窗口限额）
- 202601141229_oauth_upstreams - 上游配置增强（OpenAI base_url /v1 兼容、Codex OAuth 自动授权入库）
- 202601140645_ui-console-admin - 补齐 Web 控制台与管理后台 UI（模型列表/订阅用量/用户管理/入口）
- 202601140625_template_content_render - 修复 SSR 模板渲染与启动自举（Content 注入）
- 202601140620_mysql_migrations_multistmt - MySQL 迁移按语句拆分执行，避免 multiStatements 依赖导致的启动失败
- 202601140614_mysql_wait_ready - 开发环境启动时等待 MySQL 就绪（有限时）并重试连接
- 202601140558_mysql_autocreate_db - 开发环境 MySQL 数据库缺失时自动创建并重试连接
- 202601131914_codex - codex MVP 实现（Go 服务骨架 + 数据面代理 + SSR 控制台/管理 + MySQL 迁移 + 测试）
- 202601131951_user-system - 用户体系扩展（Web 控制台/多 Token/套餐配额对接点，已合并到 codex 方案包）
- 202601131834_codex_proxy - 旧方案包归档（已合并到最新 codex 方案包）
- 202601131834_codex_responses_relay - 旧方案包归档（已合并到最新 codex 方案包）
- 202601131731_codex_oauth_balance_research - Codex OAuth 授权与“余额/用量”口径梳理
- 202601131824_codex_cli_protocol - Codex CLI wire API 与流式协议形态确认
- 202601131722_new_api_research - new-api 的端口通信与转发链路拆解

---

## 调研：CLIProxyAPI：Codex（OpenAI）官方账号授权与“余额/用量”实现梳理

> 目的：为实现“Codex API 中转”提供可复用的鉴权链路与口径结论（以代码与官方文档为准）。

---

## 0. 结论（先说人话）

1. CLIProxyAPI 的 Codex 登录走的是 **OAuth 2.0 Authorization Code + PKCE**，回调端口固定 **1455**，与官方 Codex CLI 的约定一致。
2. 登录完成后，CLIProxyAPI 会拿到 `access_token / refresh_token / id_token`，并保存为本地 JSON（例如 `codex-<email>.json`）。
3. 代理转发 Codex 请求时，CLIProxyAPI 默认把 **`access_token` 当作 Bearer Token**，请求上游 `https://chatgpt.com/backend-api/codex/responses`，并补齐一组 **“伪装成 Codex CLI”** 的 Header。
4. **“余额/credits 查询”并未在 CLIProxyAPI 代码中实现。** 项目能直接拿到的与账户相关信息，主要来自 `id_token` 中的 `plan_type / subscription_active_*`（用于展示订阅状态），以及本地统计的 usage 日志。
5. 若你要做“用量/花费”查询，官方公开的是 **Usage / Costs API**（通常需要 Admin key）；“预付费余额/credit balance”更多是 Billing 页面概念，目前未见明确的公开查询 API 文档（需以官方最新文档为准）。

---

## 1. 上游版本信息（本次分析对象）

- 上游仓库：`router-for-me/CLIProxyAPI`
- 分析基线：commit `43652d044c5b84117aeaef90390a967e4ee29970`（2026-01-13）

---

## 2. Codex OAuth 登录：CLI 模式（最直接）

### 2.1 入口与流程

代码路径（关键文件）：

- `sdk/auth/codex.go`：`CodexAuthenticator.Login`（启动本地回调服务器、生成 URL、等待回调、换 token）
- `internal/auth/codex/oauth_server.go`：本地回调 HTTP Server（`/auth/callback` 与 `/success`）
- `internal/auth/codex/openai_auth.go`：拼 OAuth URL、请求 `https://auth.openai.com/oauth/token`
- `internal/auth/codex/pkce.go`：PKCE 生成（S256）

流程拆解：

1. 生成 `state` + `PKCE(code_verifier/code_challenge)`。
2. 启动本地回调 HTTP Server：监听 `127.0.0.1:1455`，等待 `GET /auth/callback?code=...&state=...`。
3. 拼接 OpenAI 授权链接并（可选）自动打开浏览器。
4. 用户登录后被重定向回本地回调，服务拿到 `authorization_code`。
5. 服务使用 `code_verifier` 向 `https://auth.openai.com/oauth/token` 交换 `access_token/refresh_token/id_token`。
6. 从 `id_token` 中解析出 `email` 与 `chatgpt_account_id` 等信息，落盘保存。

### 2.2 OAuth 授权 URL 细节（重要参数）

CLIProxyAPI 在 `internal/auth/codex/openai_auth.go` 中构造的关键参数包括：

- `client_id`: 固定为 `app_EMoamEEZ73f0CkXaXp7hrann`
- `redirect_uri`: `http://localhost:1455/auth/callback`
- `scope`: `openid email profile offline_access`
- `code_challenge_method`: `S256`
- 额外开关：`codex_cli_simplified_flow=true`、`id_token_add_organizations=true`、`prompt=login`

> 这些参数决定了“用 OpenAI 官方账号（ChatGPT）授权”的交互与回调行为；其中 `redirect_uri` 需要本地端口配合。

---

## 3. Codex OAuth 登录：管理面板/管理 API 模式

管理 API 登录的实现与 CLI 模式的核心区别：它不会直接在主服务端口上接 `redirect_uri`，而是通过 **本地回调转发器** 把 `1455` 的回调请求转发回管理 API 的 `/codex/callback`。

关键实现位置：

- `internal/api/handlers/management/auth_files.go`：`RequestCodexToken` + `startCallbackForwarder`
- `internal/api/server.go`：`GET /codex/callback`（把 `code/state/error` 写入 `.oauth-codex-<state>.oauth` 文件）

工作方式：

1. 管理 API 生成 `auth_url`，并在 WebUI 场景启动 `127.0.0.1:1455` 的 forwarder。
2. OpenAI 回调命中 forwarder（不关心 path），forwarder 302 到主服务的 `/codex/callback?...`。
3. 主服务落盘写入 `.oauth-codex-<state>.oauth`，后台 goroutine 读取文件并完成换 token + 保存凭据。

---

## 4. Token 保存格式（本地 JSON）

Codex token 文件结构定义在 `internal/auth/codex/token.go`（示例为占位符）：

```json
{
  "id_token": "eyJ...<redacted>",
  "access_token": "eyJ...<redacted>",
  "refresh_token": "eyJ...<redacted>",
  "account_id": "user-...<redacted>",
  "last_refresh": "2026-01-13T17:31:00Z",
  "email": "user@example.com",
  "type": "codex",
  "expired": "2026-01-14T17:31:00Z"
}
```

注意点：

- CLIProxyAPI 会把凭据以明文 JSON 保存到 `auth-dir`（默认 `~/.cli-proxy-api`），这对开发方便，但对安全要求更高（权限、加密、备份策略要想清楚）。

---

## 5. 代理转发时如何“带上授权”（真正跑起来的关键）

关键位置：

- `internal/runtime/executor/codex_executor.go`

核心逻辑：

1. 默认上游：`https://chatgpt.com/backend-api/codex`，最终请求 `POST /responses`。
2. 取凭据优先级：
   - 若配置了 `api_key`（`codex-api-key`），使用 API key
   - 否则使用 `auth.Metadata["access_token"]`（OAuth access token）
3. 注入 Header（`applyCodexHeaders`）：
   - `Authorization: Bearer <token>`
   - `Openai-Beta: responses=experimental`
   - `Originator: codex_cli_rs`（仅 OAuth 模式）
   - `Chatgpt-Account-Id: <account_id>`（仅 OAuth 模式，来自 id_token 解析）
   - 以及一组 `Version/Session_id/User-Agent` 等“模拟 Codex CLI”的字段

---

## 6. Token 刷新（refresh_token → access_token）

关键位置：

- `internal/auth/codex/openai_auth.go`：`RefreshTokens`
- `internal/runtime/executor/codex_executor.go`：`Refresh`

刷新策略：

- executor 检测到存在 `refresh_token` 时，调用 `grant_type=refresh_token` 刷新，更新 `id_token/access_token/refresh_token/expired/last_refresh`。

---

## 7. “余额/用量/配额”口径：CLIProxyAPI做了什么、没做什么

### 7.1 CLIProxyAPI做了什么

- **订阅状态展示（来自 id_token claims）**  
  `internal/api/handlers/management/auth_files.go` 的 `extractCodexIDTokenClaims` 会从 `id_token` 解析并对外暴露：
  - `chatgpt_account_id`
  - `plan_type`
  - `chatgpt_subscription_active_start / until`

- **本地 usage 统计**  
  CLIProxyAPI 有自己的 in-memory 统计（用于观测/计数），但它不是 OpenAI 账户侧的“余额”。

### 7.2 CLIProxyAPI没做什么（你可能以为它做了）

- 代码中未看到对 OpenAI Billing/Balance 类接口的调用（例如余额、credit_grants 等）。
- 也未看到对“预付费余额”的官方查询 API 封装。

---

## 8. 给你写“Codex API 中转项目”的建议（KISS）

1. **先把“余额/用量”定义清楚**：你要的是订阅状态（plan），还是 API 花费（cost），还是预付费剩余（credit balance）？这三者不是一回事。
2. **优先走官方稳定接口**：如果目标是 OpenAI API，用官方 API Key + 官方 Usage/Costs API 做用量/花费统计（权限与密钥管理要到位）。
3. **Codex OAuth 适合作为“官方账号授权”研究样本**：它能跑通 OAuth/PKCE/刷新链路，但依赖的上游行为与 Header 可能会变，务必做好兼容与降级策略。
4. **不要把明文 token 当成“无所谓”**：至少保证文件权限、日志脱敏、备份隔离；上云则必须有 KMS/密钥托管。

---

## 9. 参考（官方文档）

- Codex CLI Authentication（回调端口、token 存储、不同登录方式）：https://developers.openai.com/codex/auth
- Usage API / Costs API（用量与花费查询）：https://platform.openai.com/docs/api-reference/usage
- 预付费 Billing（余额/credit 概念与扣费方式）：https://help.openai.com/en/articles/8264644-how-can-i-set-up-prepaid-billing
- API Usage Dashboard（用量看板与口径说明）：https://help.openai.com/en/articles/10478918-api-usage-dashboard
