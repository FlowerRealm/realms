# 技术设计: Codex OAuth 对齐 CLIProxyAPI

## 技术方案

### 核心技术
- Go `net/http`：本地回调监听与管理后台交互
- OAuth2 Authorization Code + PKCE：`S256` challenge
- token 安全存储：沿用现有 AES-GCM 应用层加密入库

### 实现要点
- **对齐 authorize URL 生成**
  - 参数对齐：`client_id/scope/prompt/redirect_uri/state/code_challenge/code_challenge_method`
  - 输出形式对齐：管理后台发起时可直接跳转到 authorize URL

- **回调处理体验（HTML 页）**
  - 提供 success / error 页面（可内嵌模板字符串或复用现有模板渲染）
  - 页面内容以“可操作”为目标：成功提示、失败原因、下一步动作（重试/检查端口/重新发起授权）

- **错误分层与用户可读提示**
  - 在 `internal/codexoauth` 内引入可枚举错误码（例如：端口占用、回调超时、state 不匹配、换码失败、刷新失败）
  - 管理后台展示“用户可读消息”，同时日志保留“开发者诊断信息”（不泄露 token）

- **刷新策略对齐（lead + retry/backoff）**
  - lead：与 CLIProxyAPI 接近（默认 5 分钟量级）
  - retry：有限次数（例如 2-3 次）+ 简单退避（例如 2s/5s），只覆盖“疑似短暂错误”
  - 失败后进入 cooldown（沿用数据库字段），避免并发刷新风暴

- **关键可观测点**
  - 授权发起：记录 endpoint/account、state 创建（不记录 state 明文亦可）
  - 回调成功/失败：错误码、原因、耗时
  - 刷新成功/失败：错误码、cooldown 设置、是否进入 disabled/invalid（如有）

## 架构设计
本次变更不引入新服务，保持现有：
- 主服务（数据面/管理面）
- 本地回调监听（`127.0.0.1:1455`，仅用于 OAuth 回调）

## API设计
以兼容为原则：
- 现有管理后台“发起授权”入口保持不变（如需新增辅助入口，例如手动粘贴回调 URL，仅作为可选增强）

## 数据模型
不变更表结构，沿用：
- `codex_oauth_accounts`：`access_token_enc/refresh_token_enc/id_token_enc/expires_at/last_refresh_at/status/cooldown_until`

## 安全与性能
- **安全**
  - 绝不在日志/页面/错误中输出 `access_token/refresh_token/id_token` 明文
  - 回调 handler 严格校验 `state`，并确保一次性消费（防重放）
  - 保持 token 入库加密与最小权限原则
- **性能**
  - refresh 重试有上限 + cooldown，避免并发风暴
  - lead 窗口不宜过大，避免过度刷新

## 测试与部署
- 单元测试：PKCE 生成、authorize URL 参数、state 生命周期、刷新触发判定
- 集成/回归：模拟回调成功/失败、刷新成功/失败与 cooldown 行为
- 部署：无迁移；仅需在本机确保 `127.0.0.1:1455` 可监听

