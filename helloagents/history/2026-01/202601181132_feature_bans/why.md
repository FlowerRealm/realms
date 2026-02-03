# 变更提案: 管理后台功能禁用（Feature Bans）

## 需求背景

目前项目已存在 `self_mode`（自用模式），用于在运行时“硬禁用”计费/支付/工单等功能域。但除了 `self_mode` 之外，缺少一个统一的、可在管理后台即时生效的“功能禁用/解禁”机制。

这导致：
- 当某个功能需要临时下线（合规、风控、运维、紧急止血）时，往往只能通过改代码/改配置并重启来达成。
- 功能禁用逻辑分散（UI 入口/后端路由/相关 API），容易出现“入口隐藏了但仍可访问”的不一致。

本需求希望：在 `root` 管理员的系统设置页面（`/admin/settings`）中，提供对项目内主要功能域的 ban/解 ban；禁用后需要同时隐藏 UI 入口并在后端拒绝访问；禁用对所有用户（包括 `root`）都生效；且**不允许禁用系统设置页本身**以避免自锁。

## 产品分析

### 目标用户与场景
- **用户群体:** `root` 管理员（运维/站点管理员）
- **使用场景:**
  - 紧急下线某功能模块（避免事故扩大）
  - 合规/风控需要临时关闭某些入口
  - 自用/内测环境希望快速精简功能（与 `self_mode` 一致的“禁用大部分功能”诉求）

### 价值主张与成功指标
- **价值主张:** 在一个稳定入口（`/admin/settings`）集中管理“功能禁用”，并保证 UI/后端行为一致、即时生效。
- **成功指标:**
  - 管理员在页面保存后无需重启即可生效（以页面刷新后可验证为准）
  - 被禁用功能的所有入口（侧边栏/关键按钮）默认不可见
  - 被禁用功能的相关页面/操作请求被后端拒绝（统一返回 404 以与现有 `self_mode` 行为保持一致）
  - 不会因为误操作导致无法从 UI 恢复（系统设置页永远可访问；“恢复默认”可清理禁用项）

### 人文关怀
避免把“禁用”做成隐性黑盒：在设置页清晰提示禁用的影响范围，并对“不可禁用项”（系统设置）明确标注原因，减少误操作与排障成本。

## 变更内容

1. 在管理后台系统设置页（`/admin/settings`）新增「功能禁用」分区，提供对主要功能域的禁用开关。
2. 后端增加统一 Feature Gate：被禁用的功能相关路由与动作统一返回 404（同时保持 `self_mode` 的硬禁用语义）。
3. 用户控制台与管理后台的 UI（侧边栏与关键入口）根据 Feature Gate 隐藏被禁用功能的入口。
4. `self_mode` 作为“硬禁用基线”：其禁用项不允许被 UI 打开（UI 仅展示为“自用模式强制禁用”）。
5. 系统设置页（`/admin/settings`）本身不可禁用（UI 不提供开关，后端也忽略相关配置）。

## 影响范围

- **模块:**
  - `internal/store`（新增 feature ban 的 `app_settings` key 常量/读写）
  - `internal/middleware`（新增 feature gate 中间件）
  - `internal/server`（路由接入 feature gate）
  - `internal/web` + `internal/web/templates`（用户侧入口隐藏与访问拒绝）
  - `internal/admin` + `internal/admin/templates`（管理侧入口隐藏、设置页读写）
- **文件（预期）:**
  - `internal/store/app_settings.go`
  - `internal/middleware/*`
  - `internal/server/app.go`
  - `internal/web/templates/base.html`（以及必要的入口页模板）
  - `internal/admin/server.go`
  - `internal/admin/templates/settings.html`
  - `helloagents/wiki/data.md`、`helloagents/wiki/modules/realms.md`（文档同步）
- **API:** 无新增对外 API；仅扩展 `/admin/settings` 的表单字段与行为。
- **数据:** 复用 `app_settings`，新增若干 `feature_disable_*` 键（bool）。

## 核心场景

### 需求: 功能禁用开关
**模块:** admin/web/server

#### 场景: 在系统设置禁用用户侧“对话”
条件：`root` 在 `/admin/settings` 勾选“禁用对话”，保存成功。
- 预期结果：用户侧侧边栏不再展示“对话”入口；`GET /chat`、`POST /api/chat/token` 等相关请求返回 404。

#### 场景: 在系统设置禁用“计费/支付”
条件：`root` 在 `/admin/settings` 勾选“禁用计费/支付”，保存成功。
- 预期结果：用户侧订阅/充值/支付相关页面与操作返回 404；管理侧订阅套餐/订单/支付渠道页面与操作返回 404；相关 webhook 入口返回 404。

#### 场景: 在系统设置禁用“工单”
条件：`root` 在 `/admin/settings` 勾选“禁用工单”，保存成功。
- 预期结果：用户侧 `/tickets*` 与管理侧 `/admin/tickets*` 返回 404；入口隐藏。

### 需求: self_mode 兼容
**模块:** server/web/admin

#### 场景: self_mode=true 时的硬禁用
条件：配置文件开启 `self_mode.enable=true`。
- 预期结果：计费/支付/工单等既有禁用项保持当前行为（无需 UI 配置即可禁用）；设置页仅展示其为“自用模式强制禁用”，不允许在 UI 中打开。

### 需求: 安全保护（避免自锁）
**模块:** admin

#### 场景: 不允许禁用系统设置页本身
条件：管理员在 UI 中无法找到“禁用系统设置页”的开关（或试图构造请求提交）。
- 预期结果：`/admin/settings` 始终可访问；后端忽略对该项的禁用写入。

### 需求: 恢复默认
**模块:** admin/store

#### 场景: 一键恢复为默认（清理禁用项）
条件：`root` 在 `/admin/settings` 点击“恢复为配置文件默认”。
- 预期结果：清理 `app_settings` 中的 feature ban 覆盖项；功能恢复（除 `self_mode` 硬禁用项外）。

## 风险评估

- **风险:** 禁用范围不一致（只隐藏 UI 但后端仍可访问）。
  - **缓解:** 以“路由级 feature gate”为 SSOT，同时让模板使用同一份 feature 状态数据。
- **风险:** 误操作导致无法从 UI 恢复（自锁）。
  - **缓解:** 系统设置页不可禁用；提供“恢复默认”清理所有禁用项；必要时可直接在 DB 删除 `app_settings` 对应键作为救援手段。

