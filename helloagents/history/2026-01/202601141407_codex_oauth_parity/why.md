# 变更提案: Codex OAuth 对齐 CLIProxyAPI

## 需求背景
Realms 已实现 Codex OAuth（PKCE + 本地回调 + token 入库/刷新），但与参考项目 CLIProxyAPI 在**错误分层、回调用户体验、刷新稳健性**等细节上仍存在差异。

本次变更目标是让 Realms 的 Codex OAuth 行为与 CLIProxyAPI 的 Codex OAuth 逻辑保持一致/相近，从而：
1. 降低实现分叉带来的维护成本与回归风险
2. 提升授权与刷新流程的可诊断性（用户可理解的错误）
3. 提升刷新/回调在边界条件下的成功率（短暂网络抖动、偶发失败）

## 变更内容
1. Codex OAuth 回调页：补齐成功/失败的 HTML 页面与用户指引（对齐 CLIProxyAPI 的 UX 预期）
2. 刷新策略：对齐“临期刷新窗口（lead）”与“刷新重试/退避”策略，避免偶发失败导致账号长期不可用
3. 错误分层：为 OAuth 授权、回调、换取 token、刷新 token 建立可枚举错误码与用户可读提示，便于管理后台展示
4. 可观测性：补齐关键点日志（不泄露敏感信息），方便定位问题（端口占用、state 不匹配、回调超时等）

## 影响范围
- **模块:**
  - `internal/codexoauth`（OAuth flow / 回调处理 / 刷新）
  - `cmd/codex`（回调监听启动行为与失败策略）
  - `internal/upstream`（请求执行与刷新触发/退避）
  - `internal/admin`（管理后台发起授权与错误提示/状态展示）
- **文件:** 预计涉及 5-10 个 Go 文件 + 1-2 个 HTML 模板 + 文档更新
- **API:** 不新增对外数据面接口；可能新增/调整管理后台内部接口或页面交互（保持兼容）
- **数据:** 不变更表结构（沿用 `codex_oauth_accounts`），仅调整状态/字段更新策略

## 核心场景

### 需求: OAuth Flow Parity
**模块:** codex_oauth / 管理后台
对齐授权 URL 生成、state 校验、PKCE 交换 code、回调落库与用户回调页体验。

#### 场景: Admin Start OAuth
管理后台对某个 Endpoint 发起 Codex OAuth 授权。
- 预期结果: 生成与 CLIProxyAPI 一致的 authorize URL（client_id/scope/prompt/redirect_uri + PKCE + state）
- 预期结果: 在回调监听不可用（端口占用/未启用）时给出明确可操作提示

#### 场景: OAuth Callback Success
用户在浏览器完成登录后跳转到本地回调。
- 预期结果: 校验 state，交换 code→token，解析 account/email，写入/更新 `codex_oauth_accounts`
- 预期结果: 返回可关闭的成功页（HTML），并提示“可回到管理后台查看状态”

#### 场景: OAuth Callback Error
回调返回 error / state 不匹配 / code 缺失。
- 预期结果: 返回失败页（HTML），包含用户可理解的错误与下一步建议

### 需求: Token Refresh Robustness
**模块:** upstream executor / store
对齐临期刷新与短暂失败重试，降低“偶发失败导致长期不可用”的概率。

#### 场景: Near-Expiry Auto Refresh
access_token 临近过期时自动 refresh。
- 预期结果: 使用与 CLIProxyAPI 接近的 lead 窗口触发刷新
- 预期结果: 刷新成功后更新 `expires_at/last_refresh_at/*_token_enc`

#### 场景: Refresh Failure Cooldown
刷新遇到临时错误（网络抖动、5xx）。
- 预期结果: 进行有限次数重试（带退避），仍失败则进入 cooldown（避免风暴）
- 预期结果: 管理后台可看到明确状态与建议（如稍后重试）

### 需求: User-Friendly Errors & Diagnostics
**模块:** codexoauth / admin
把“看不懂的错误”变成可操作的提示（对齐 CLIProxyAPI 的错误体验）。

#### 场景: Callback Port Busy
回调监听端口被占用。
- 预期结果: 立即提示端口占用（端口/进程提示可选），不让用户走到回调失败才发现

## 风险评估
- **风险:** 调整刷新窗口/策略可能改变刷新触发频率
  - **缓解:** 保持原有 cooldown/状态机制；新增重试次数上限；默认值可控
- **风险:** 回调监听失败时的启动策略调整可能影响启动行为
  - **缓解:** 仅在 `codex_oauth.enable=true` 时严格校验；提供清晰日志与配置指引

