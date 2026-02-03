# 技术设计: OAuth Apps（授权码换取 Realms API Token）

## 技术方案

### 核心技术
- OAuth 2.0/2.1 风格的 Authorization Code Flow（带 state 防 CSRF）
- 可选 PKCE（面向纯前端/移动端时应支持；对用户透明）
- 现有鉴权复用：
  - Web 登录态：Cookie Session（`realms_session`）
  - 数据面调用：`Authorization: Bearer rlm_...`（`user_tokens`）

### 实现要点（推荐路径）

1. **应用模型（OAuth Apps）**
   - `oauth_apps`: `client_id`、`name`、`status`、`client_secret_hash`（可选）
   - `oauth_app_redirect_uris`: `app_id` + `redirect_uri`（精确匹配）
   - 可选：`oauth_app_scopes`（允许的 scope 列表；先做最小集）

2. **授权码**
   - `oauth_auth_codes`: `code_hash`、`app_id`、`user_id`、`redirect_uri`、`scope`、`expires_at`、`consumed_at`
   - 只存 hash，不存明文 code；明文仅通过重定向返回一次
   - TTL 建议 5-10 分钟；消费后不可复用

3. **用户同意与记住授权**
   - `oauth_user_grants`: `user_id`、`app_id`、`scope`、`created_at/updated_at`
   - `GET /oauth/authorize`：
     - 校验 `client_id`、`response_type=code`、`redirect_uri`（必须在白名单）
     - 若未登录：重定向 `/login?next=...`（站内安全回跳）
     - 若已存在 grant 且 scope 满足：可跳过确认直接发码
     - 否则渲染同意页（显示 app 名称/权限/回调域名），提交到 `POST /oauth/authorize`
   - `POST /oauth/authorize`（CSRF 保护）：
     - 校验用户确认
     - 写入/更新 grant（若勾选“记住授权”）
     - 生成授权码并回跳带 `code` + `state`

4. **Token 交换**
   - `POST /oauth/token`：
     - 校验 `grant_type=authorization_code`、`code`、`client_id`、`redirect_uri`
     - 若该 app 配置了 secret：要求 `client_secret`（与 hash 比较）
     - 若使用 PKCE：校验 `code_verifier`（与 `code_challenge` 匹配）
     - 生成 `rlm_...` 并写入 `user_tokens`（name 建议 `oauth:<client_id>` 或 `oauth:<app_id>`）
     - 建议新增映射表 `oauth_app_tokens` 记录 `token_id` 与 `app_id/user_id/scope`，便于撤销

5. **撤销**
   - 最小可用：用户在 `/tokens` 页面可看到并撤销 `oauth:*` 名称的 Token
   - 增强：新增“已授权应用”页展示每个 app 的 token 列表并一键撤销（后续迭代）

6. **登录回跳（next 参数）**
   - 现有 `/login` 成功后固定跳转 `/dashboard`，无法承载 OAuth 流程
   - 需要为 `GET/POST /login` 增加 `next`（安全校验：仅允许站内路径）

## 架构决策 ADR

### ADR-001: access_token 直接使用 Realms 现有 `user_tokens`（`rlm_...`）
**上下文:** 需求明确要求“授权成功拿 API Token”，并希望外部客户端可直接调用 `/v1/*`。  
**决策:** `POST /oauth/token` 直接签发 `rlm_...` 并落库到 `user_tokens`。  
**理由:** 复用现有鉴权链路，改动最小且可立即用于数据面。  
**替代方案:** 自建短期 JWT/opaque token + refresh token → 拒绝原因: 会引入第二套鉴权与续期体系，复杂且易出错。  
**影响:** 初期 token 可能为长期有效，需要配套撤销与（后续）scope/过期控制。

### ADR-002: redirect_uri 采用精确匹配白名单
**上下文:** redirect_uri 是 OAuth 最关键的安全边界。  
**决策:** 每个 app 登记 redirect_uri 列表，authorize/token 均要求与列表精确匹配。  
**理由:** 避免开放重定向与 token 泄露。  
**替代方案:** 仅校验同域/前缀匹配 → 拒绝原因: 容易被路径注入/子域/编码绕过。  

## API 设计

### [GET] /oauth/authorize
- **输入:** `response_type=code`、`client_id`、`redirect_uri`、`scope`、`state`（可选 `code_challenge`/`code_challenge_method`）
- **行为:** 登录校验 + 授权同意页

### [POST] /oauth/authorize
- **认证:** Cookie Session + CSRF
- **行为:** 生成授权码并回跳 `redirect_uri`

### [POST] /oauth/token
- **请求:** `application/x-www-form-urlencoded`
- **输入:** `grant_type=authorization_code`、`code`、`client_id`、`redirect_uri`（可选 `client_secret` / `code_verifier`）
- **响应:** `access_token`（`rlm_...`）、`token_type=bearer`（可选 `scope`）

## 数据模型（建议）

```sql
-- oauth_apps / oauth_app_redirect_uris / oauth_auth_codes / oauth_user_grants / oauth_app_tokens
```

## 安全与性能

- 关键安全点：state 校验、redirect_uri 精确匹配、code 单次使用 + 过期、token 不入日志
- CSRF：同意页 POST 必须走 CSRF 中间件
- 登录回跳（next）：仅允许站内相对路径；后端拒绝 `//`/绝对 URL，避免 open redirect
- 错误回跳策略：当 `redirect_uri` 未登记/不可信时直接返回 400（不回跳），避免泄露 code/token
- 敏感信息：`client_secret` 仅存 hash（bcrypt）；授权码仅存 hash；访问日志不记录 query string
- 速率限制（后续）：对 `/oauth/token` 与 `/oauth/authorize` 可加简单限流（非本期强制）

## 测试与部署

- 单元/集成测试覆盖：
  - redirect_uri 不匹配拒绝
  - 未登录 → /login → 回跳继续授权
  - 授权码单次使用/过期
  - token 交换成功后可调用 `/v1/models`（鉴权为 token）
- 数据库迁移：新增 oauth 表；不改动存量用户与 token 结构（仅新增映射表）
