# 技术设计: 上游配置增强（OpenAI API Key / Codex OAuth 自动授权）

## 技术方案

### 1) OpenAI 兼容上游 base_url 规范化

问题根因：现有实现将下游路径（`/v1/*`）直接 Join 到 base_url；当 base_url 已包含 `/v1` 时会出现 `/v1/v1/*`。

方案：
- 解析并规范化 base_url 的 path：
  - 若 base_url 的 path（去掉末尾 `/`）以 `/v1` 结尾，且目标路径以 `/v1/` 开头，则从目标路径移除前缀 `/v1`。
- 保持兼容：未包含 `/v1` 的 base_url 行为不变。

### 2) Codex OAuth 自动授权闭环

#### 2.1 发起授权（Admin）
- 在 Codex Accounts 页面提供按钮：`POST /admin/endpoints/{endpoint_id}/codex-oauth/start`
- 服务端生成：
  - `state`（随机）
  - `code_verifier`（随机）
  - `code_challenge`（S256）
- 将 `state → {endpoint_id, code_verifier, created_at, actor_user_id}` 暂存在内存（TTL 10 分钟）。
- 302 重定向到 OpenAI 授权地址（包含 PKCE/state/scope/redirect_uri）。

#### 2.2 回调接收（本机 1455）
由于 Codex OAuth 的 redirect_uri 通常固定为 `http://localhost:1455/auth/callback`（与 Codex CLI 约定一致），实现方式：
- 主服务启动时额外监听 `127.0.0.1:1455` 的最小回调 server（仅处理 `/auth/callback` 与 `/success`）。
- 回调收到 `code/state` 后：
  1. 校验 state 命中且未过期
  2. 请求 token endpoint 换取 `access_token/refresh_token/id_token`
  3. 从 `id_token` 解析 `chatgpt_account_id/email/plan_type/subscription_active_*`
  4. 加密入库到 `codex_oauth_accounts`（含 id_token_enc）
  5. 返回一个简短 HTML 提示页，给出“返回管理后台”的链接

#### 2.3 参数与可配置项
- 默认采用 Codex CLI OAuth 的关键参数（client_id/authorize/token endpoint/scope 等）
- 提供配置覆盖（后续可扩展到自定义 client_id）

### 3) 管理后台 UI 调整
- Codex Accounts 页面新增：
  - “发起授权”按钮
  - 授权说明与错误提示
- 现有订阅字段展示逻辑复用（从 id_token claims 提取）。

## 安全与性能

- state 仅内存存储且短 TTL；回调 server 仅绑定 `127.0.0.1`。
- 不记录明文 token；UI 仅展示非敏感字段与状态。
- token 交换与回调处理增加超时，避免卡住管理线程。

## 测试与验证

- 单元测试：
  - base_url path 规范化（包含 `/v1` 与不包含两类）
  - PKCE challenge 生成（格式正确）
- 手动验证：
  - admin 配置 openai_compatible：base_url 可填 `.../v1`，模型列表可用
  - codex_oauth 走完整 OAuth 流程并成功入库

