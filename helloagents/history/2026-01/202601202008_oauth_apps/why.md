# 变更提案: OAuth Apps（外部客户端授权登录并获取 Realms API Token）

## 需求背景

当前 Realms 已提供：
- Web 控制台账号体系（邮箱/账号名/密码 + Cookie Session）
- 数据面 API（`/v1/responses`、`/v1/chat/completions`、`/v1/models`），通过 `Authorization: Bearer rlm_...` 鉴权

但“外部对话客户端/第三方应用”若想让用户使用 Realms 账号登录并调用数据面 API，目前只能让用户手工复制粘贴 Token，体验与安全性都不理想。

本需求希望新增类似 GitHub OAuth Apps 的能力：外部应用跳转到 Realms 站点完成登录与授权，授权后回调到应用并返回一个可用的 Realms API Token（`rlm_...`），用于调用 Realms 的 `/v1/*`。

## 产品分析

### 目标用户与场景
- **普通用户**
  - 在外部对话客户端点击“使用 Realms 登录”
  - 跳转到 Realms 完成登录并同意授权
  - 回到客户端后即可对话（客户端使用授权得到的 API Token 调用 Realms）
- **管理员（root）**
  - 在管理后台创建/管理 OAuth 应用（client_id、client_secret、回调地址白名单、显示名称等）
  - 可查看/撤销应用的授权（最小可用版本可先只支持用户侧撤销）

### 价值主张与成功指标
- **价值主张**
  - 降低集成门槛：外部客户端无需引导用户手工配置 Token
  - 提升安全性：回调地址白名单、state 防 CSRF、一次性授权码交换 Token、可撤销
  - 为后续“统一生态”（类似 GitHub 登录/授权）打基础：应用管理、scope、授权记录
- **成功指标（可验收）**
  - 外部应用可通过 OAuth 授权拿到 `rlm_...` 并可直接调用 `/v1/responses`
  - 用户授权必须经过明确同意（展示应用名与权限范围）
  - redirect_uri 必须严格匹配该应用登记白名单（防开放重定向/Token 泄露）
  - 授权码一次性、短期有效；重复使用/过期必须失败
  - 用户可在 Web 控制台撤销该应用已发放的 Token（或撤销授权）

## 变更内容

1. 新增 OAuth Provider（Authorization Server）能力：
   - 授权端点（Authorize）：用户登录后展示授权同意页，成功后回跳带 `code` + `state`
   - 令牌端点（Token）：外部应用用 `code` 交换得到 Realms API Token（`rlm_...`）
2. 新增 OAuth Apps 管理（管理后台）：
   - 创建/编辑/禁用应用
   - 配置回调地址白名单（redirect_uri 列表）
   - 生成/轮换 client_secret（如支持机密客户端）
3. 新增用户授权记录（最小集）：
   - 记录用户对某应用的授权（用于“记住授权/跳过重复确认”与后续撤销）
   - 记录 OAuth 发放的 Token 归属（便于按应用撤销）

## 影响范围

- **模块:**
  - `internal/server/*`（路由注册）
  - `internal/web/*`（登录流程增加 next 回跳；授权同意页模板）
  - `internal/admin/*`（OAuth Apps 管理页）
  - `internal/store/*` + `internal/store/migrations/*`（OAuth Apps/redirect_uris/auth_codes/grants/tokens 映射）
  - `internal/middleware/*`（可选：scope 校验/未来扩展）
- **API:**
  - 新增：`GET /oauth/authorize`、`POST /oauth/authorize`、`POST /oauth/token`（以及可选的 revoke/introspect）
  - 管理后台：`/admin/oauth-apps*`

## 核心场景

### 需求: 外部应用发起授权并回调成功
**模块:** oauth/web/store

#### 场景: 用户完成登录并同意授权
外部应用发起跳转到：
`/oauth/authorize?response_type=code&client_id=...&redirect_uri=...&scope=...&state=...`
- 未登录：跳转到 `/login`，登录成功后回到原 authorize 请求
- 已登录：展示“授权同意”页（应用名称 + 权限范围）
- 同意后：302 回跳到 `redirect_uri?code=...&state=...`

#### 场景: 外部应用用授权码换取 API Token
外部应用以表单请求调用 `POST /oauth/token` 交换：
- code 必须一次性、短期有效
- 成功返回 `access_token=rlm_...`（可直接用于 `Authorization: Bearer` 调用 `/v1/*`）

### 需求: redirect_uri 安全校验
**模块:** oauth/store

#### 场景: 非白名单回调必须拒绝
当 `redirect_uri` 未严格匹配该 app 登记的 URI 列表时：
- `GET /oauth/authorize` 必须拒绝并给出明确错误（不得回跳到不可信地址）

## 风险评估

- **风险:** OAuth 引入新的攻击面（开放重定向、code 注入、CSRF、token 泄露）  
  **缓解:** redirect_uri 白名单精确匹配；state 必填校验；授权码单次使用 + 过期；token 仅在后端返回；日志中禁止输出 code/token。
- **风险:** 登录回跳（next）引入开放重定向  
  **缓解:** next 仅允许站内相对路径或同源白名单；否则回退到 `/dashboard`。
- **风险:** token 长期有效导致外部应用泄露后影响扩大  
  **缓解:** 为 OAuth token 命名并可一键撤销；后续可引入 scope 与到期时间（非本期强制）。

