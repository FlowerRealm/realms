# 技术设计: Codex OAuth 对齐 CLIProxyAPI

## 技术方案

### 核心技术
- OAuth 2.0 Authorization Code + PKCE（S256）
- Go `net/http`（回调监听/管理后台 handler）
- JWT claims（不做签名校验，仅用于提取展示字段；token 真正有效性由 OAuth 服务端保证）
- token 安全存储：沿用现有 AES-GCM 应用层加密入库

### 实现要点

#### 1) 结构化解析 `id_token`（对齐 CLIProxyAPI）
- 在 `internal/codexoauth` 内提供结构体 claims 解析（参考 CLIProxyAPI `internal/auth/codex/jwt_parser.go`）：
  - 顶层：`email`
  - 嵌套：`https://api.openai.com/auth` → `chatgpt_account_id / chatgpt_plan_type / chatgpt_subscription_active_*`
- 提供“兼容兜底”：
  - 若嵌套 claims 不存在，则回退到当前的若干备用字段（如 `chatgpt_account_id` 顶层等）

#### 2) 统一 claims 解析入口，避免重复实现
- 复用 `internal/codexoauth` 的解析逻辑：
  - OAuth 回调落库：用统一解析结果获取 `account_id/email`
  - 管理后台展示：用统一解析结果展示 `plan_type/subscription_active_*`
- 移除或收敛 `internal/admin` 中的 JWT 解析重复代码（避免两份逻辑不一致）

#### 3) 管理后台 UX：明确 `localhost:1455` 的前置条件
- 在 `Codex OAuth Accounts` 页面新增提示区块：
  - 说明 `redirect_uri` 固定为 `http://localhost:1455/auth/callback`
  - 给出 SSH 端口转发命令示例（用户在本机执行）
- （可选增强）增加“粘贴回调 URL”表单：
  - 允许用户把浏览器地址栏的回调 URL/Query 粘贴回来
  - 服务端解析出 `code/state/error` 后复用同一套 state 校验与 token 交换逻辑完成入库

## 架构设计
保持现有架构不变：
- 主服务（管理面板/数据面）
- OAuth 回调监听（默认 `127.0.0.1:1455`，仅用于 `GET /auth/callback`）

## API设计（管理后台内部）
可选增强需要新增一个管理后台内部路由：
- `POST /admin/endpoints/{endpoint_id}/codex-oauth/complete`（或同等路径）
  - 入参：`callback_url`（或 `code/state`）
  - 行为：解析并完成授权（等价于回调 handler 的“处理核心”）

## 数据模型
不变更表结构，继续使用：
- `codex_oauth_accounts`：`account_id/email/access_token_enc/refresh_token_enc/id_token_enc/expires_at/last_refresh_at/status/cooldown_until`

## 安全与性能
- **安全**
  - 严禁在日志/页面/错误中输出 `access_token/refresh_token/id_token` 明文
  - 回调与手动完成均需校验 `state`（一次性消费 + TTL）
  - 权限：仅 root 可发起/完成 OAuth，沿用现有管理后台保护
- **性能**
  - claims 解析仅对 `id_token` 做 base64 解码与 JSON 反序列化，开销可忽略

## 测试与部署
- 单元测试（优先）：
  - `id_token` claims 解析：嵌套 claims/兜底分支
  - （可选）回调 URL 解析：支持完整 URL / query / fragment
- 部署：
  - 无 DB 迁移
  - 需要确保 `codex_oauth.callback_listen_addr` 可监听（默认 `127.0.0.1:1455`）

