# 变更提案: 自用模式（禁用计费与工单）

## 需求背景
当前 Realms 提供了完整的“多用户 + 订阅/订单/充值/支付 + 工单”等功能域，用于通用部署场景。
但在“自用（单人/小圈子）”场景下，这些功能会带来不必要的复杂度与攻击面（更多路由、更多配置项、更多页面入口、更多状态机与边界条件）。

本提案希望新增一个明确的 **自用模式（self_mode）**：在不破坏默认行为的前提下，在该模式下硬禁用一批不需要的功能域，使服务更简单、更安全、更“少打扰”。

## 产品分析

### 目标用户与场景
- **用户群体:** 自行部署 Realms 的个人用户（主要是仓库维护者本人）
- **使用场景:** 仅需要 OpenAI 兼容 API 中转 + 基础管理（渠道/模型/用户/用量），不需要对外售卖订阅、不需要支付回调、不需要用户工单
- **核心痛点:** 配置与页面入口过多；计费与工单相关代码与路由增加维护成本与误操作风险

### 价值主张与成功指标
- **价值主张:** “默认不变，自用更干净”——通过模式开关一键关闭不需要的业务域
- **成功指标:**
  - 自用模式开启后：计费/支付/工单相关路由全部不可达（404/明确禁用）
  - 自用模式开启后：OpenAI 兼容数据面仍可正常转发（不再被“订阅未激活”阻断）
  - 自用模式关闭（默认）：现有行为完全不变（Never Break Userspace）

### 人文关怀
自用模式默认不引入额外的数据采集；仅在需要的范围内保留用量记录用于自查与排障，避免为了“自用”而牺牲可观测性与可控性。

## 变更内容
1. 新增配置开关 `self_mode.enable`（默认 `false`）
2. 自用模式开启时，禁用以下功能域（路由层硬禁用 + UI 隐藏入口）：
   - 订阅/套餐（plans / user_subscriptions）
   - 订单（subscription_orders / topup_orders）
   - 支付（payment_channels + Stripe/EPay webhook + 支付页）
   - 工单（tickets：用户端与后台端、附件上传/下载、清理任务）
3. 自用模式下的数据面配额策略调整：
   - 不再要求“订阅已激活”
   - 仍可记录用量事件（用于 Usage 页面/排障），但不做余额扣费/订阅扣费

## 影响范围
- **模块:**
  - `internal/config`（新增 self_mode 配置）
  - `internal/server`（路由注册按模式裁剪；可选停止 tickets 清理 loop）
  - `internal/quota`（新增“自用模式配额 Provider”，绕开订阅/余额硬约束）
  - `internal/web` / `internal/admin`（导航与页面入口隐藏；可选显示“已禁用”提示）
  - `helloagents/wiki`（API 与模块文档补充模式差异）
- **文件:** 预计修改 8-15 个文件（以路由+模板+quota 为主）
- **API:**
  - 不新增新路径
  - 自用模式下：计费/支付/工单相关路径返回 404/禁用；数据面不再返回 `订阅未激活`
- **数据:** 不新增迁移；沿用现有表结构

## 核心场景

### 需求: self_mode 开关
**模块:** realms

#### 场景: enable_self_mode
在配置中开启 `self_mode.enable=true` 后：
- 计费/支付/工单相关页面与接口不可访问
- 其余核心功能（/v1/*、渠道/模型/用户管理、用量查询）仍可用

#### 场景: default_mode_unchanged
保持 `self_mode.enable=false`（默认）：
- 所有现有功能与行为保持不变

### 需求: 禁用计费与支付域
**模块:** realms

#### 场景: disable_billing_routes
开启自用模式后：
- `/subscription`、`/topup`、`/pay/*`、支付 webhook、后台订阅/订单/支付渠道相关入口均不可达

### 需求: 禁用工单域
**模块:** realms

#### 场景: disable_tickets_routes_and_jobs
开启自用模式后：
- `/tickets*` 与 `/admin/tickets*` 不可达
- 附件上传/下载入口不可达；可选停止附件清理后台任务

### 需求: 自用模式配额策略
**模块:** realms

#### 场景: allow_api_without_subscription
开启自用模式后：
- `/v1/responses` 与 `/v1/chat/completions` 不再因“订阅未激活”直接拒绝请求
- 仍可在数据库中记录 `usage_events`（便于 Usage 页面与排障）

## 风险评估
- **风险:** 只隐藏 UI 不禁用路由，导致“功能仍可被直接调用”
  - **缓解:** 以 `internal/server/app.go` 的路由注册作为第一道硬开关（未注册即 404）
- **风险:** 改动 quota 策略影响默认模式
  - **缓解:** 仅在 `self_mode.enable=true` 时切换 Provider；默认仍使用现有 HybridProvider
- **风险:** 文档与行为不一致
  - **缓解:** 同步更新 `helloagents/wiki/api.md` 与 `helloagents/wiki/modules/realms.md`，并在 Changelog 记录

