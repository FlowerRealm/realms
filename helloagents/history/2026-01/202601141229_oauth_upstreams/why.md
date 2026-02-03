# 变更提案: 上游配置增强（OpenAI API Key / Codex OAuth 自动授权）

## 需求背景

当前管理后台支持配置上游（channel/endpoint/credential/account），但在实际使用中仍存在关键缺口：

1. **自定义 base_url 的 OpenAI 兼容上游**在常见配置写法下不可用（例如 base_url 写成 `.../v1` 会导致路径拼接错误），容易被误判为“API key 不生效”。
2. **Codex OAuth 上游**目前仅支持手工录入 token，缺少“生成授权链接 → 登录 → 回调换 token → 自动入库”的闭环，使用成本高且容易出错。

目标是把“能跑”变成“好用”，并把上游接入的关键路径一次性补齐。

## 变更内容

1. OpenAI 兼容上游：支持 base_url 既可填 `https://api.openai.com` 也可填 `https://api.openai.com/v1`（或同类自定义 URL），避免重复 `/v1` 导致的请求失败。
2. Codex OAuth 上游：在管理后台提供自动授权入口：
   - 生成 PKCE + state
   - 引导用户打开 OpenAI 授权页
   - 回调后自动向 OpenAI 交换 token（`access_token/refresh_token/id_token`）
   - 从 `id_token` 提取 `account_id/email/订阅字段`，加密入库到 `codex_oauth_accounts`
3. 管理后台 UI：补齐 Codex OAuth Accounts 页面上的“发起授权”入口与回调结果提示。

## 影响范围

- **模块:**
  - `internal/upstream`（请求 URL 拼接规则）
  - `internal/admin`（OAuth 发起与回调落库、UI）
  - `internal/server`（路由/启动附加回调监听）
  - `internal/config`（可选：新增 OAuth 配置）
- **文件:**
  - `internal/upstream/executor.go`
  - `internal/admin/server.go`
  - `internal/admin/templates/codex_accounts.html`
  - `internal/server/app.go`
  - `internal/config/*`
  - `helloagents/wiki/api.md`
  - `helloagents/CHANGELOG.md`

## 核心场景

### 需求: 自定义 URL + API Key 可用
**模块:** upstream/admin

#### 场景: base_url 带 /v1 的 OpenAI 兼容上游仍可正常调用
管理员将 endpoint base_url 配置为 `https://api.openai.com/v1` 并添加 API key，系统应正确请求 `/v1/models`、`/v1/responses` 等，不应出现 `/v1/v1/*` 这类路径错误。

### 需求: Codex OAuth 自动授权闭环
**模块:** admin

#### 场景: 管理后台生成授权链接并自动入库
管理员在 `/admin/endpoints/{endpoint_id}/codex-accounts` 点击“生成授权链接”，完成登录后系统自动入库账号与 token，并在列表中可见。

#### 场景: 订阅状态可见
自动入库后，列表可展示 `plan_type/subscription_active_*`（若存在），用于快速判断账号订阅状态。

## 风险评估

- **风险:** OAuth 回调属于敏感路径，可能被滥用。  
  **缓解:** state 绑定 endpoint_id/操作者会话并设置短 TTL；仅允许本机回调（127.0.0.1）；回调只写入目标 endpoint；不输出明文 token。
- **风险:** OAuth 细节/参数上游可能变更。  
  **缓解:** 将关键参数做成可配置项，并在 UI/日志中提供清晰失败信息（不含敏感字段）。

