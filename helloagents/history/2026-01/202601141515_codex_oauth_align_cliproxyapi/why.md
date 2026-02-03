# 变更提案: Codex OAuth 对齐 CLIProxyAPI

## 需求背景
Realms 已实现 Codex OAuth（Authorization Code + PKCE + 本地回调 1455 + token 加密入库 + 自动 refresh）。但与参考项目 `router-for-me/CLIProxyAPI` 的 Codex OAuth 逻辑相比，Realms 目前在 **ID token claims 解析与管理后台可用性/提示** 上存在差异，可能导致：

1. `chatgpt_account_id` 解析失败或不稳定（影响上游请求头 `Chatgpt-Account-Id`，进而影响 Codex 上游可用性）
2. 管理后台对 `plan_type / chatgpt_subscription_active_*` 的展示不准确（难以判断账号订阅状态）
3. 远程访问管理后台时，用户不清楚 `localhost:1455` 回调的前置条件（SSH 端口转发/手动粘贴回调 URL 等）

本次目标：让 Realms 的 Codex OAuth 行为在关键点上与 CLIProxyAPI 对齐，优先保证 **可用性与可诊断性**，避免引入不必要复杂度。

## 变更内容
1. **ID token claims 对齐解析**
   - 解析 `id_token` 中的 `https://api.openai.com/auth` 嵌套 claims（对齐 CLIProxyAPI 的 `codex.ParseJWTToken`）
   - 提取并落库 `chatgpt_account_id`（作为 `account_id`）与 `email`
2. **管理后台展示对齐**
   - 从 `id_token` 的嵌套 claims 提取并展示 `plan_type`、`chatgpt_subscription_active_start`、`chatgpt_subscription_active_until`
3. **管理后台可操作提示（对齐 CLIProxyAPI 的 UX 预期）**
   - 在 Codex Accounts 页增加对 `localhost:1455` 回调的说明（SSH 端口转发命令示例）
   - 可选增强：提供“粘贴回调 URL”完成授权（无本地转发时的兜底路径）

## 影响范围
- **模块:**
  - `internal/codexoauth`（claims 解析、授权回调落库）
  - `internal/admin`（管理后台展示与可操作提示、可选手动完成入口）
- **文件:** 预计 4-8 个文件（Go + HTML + 测试 + 文档）
- **API:** 仅管理后台内部路由可能新增（可选手动完成）；数据面对外接口不变
- **数据:** 不变更表结构；继续使用 `codex_oauth_accounts` 的加密字段

## 核心场景

### 需求: ID Token Claims 对齐
**模块:** codexoauth / admin
对齐 `id_token` 的 claims 解析，确保 `account_id/plan_type/subscription_active_*` 可被稳定提取。

#### 场景: OAuth Callback Success（claims 解析）
用户完成登录后回调到 `http://localhost:1455/auth/callback`。
- 预期结果: 从 `id_token` 解析出 `chatgpt_account_id` 并作为 `account_id` 入库
- 预期结果: 管理后台可展示 `plan_type` 与订阅有效期字段（来自 `id_token`）

#### 场景: OAuth Callback Error（claims 缺失/结构变化）
回调成功但 `id_token` 中缺少预期字段，或结构发生变化。
- 预期结果: 给出用户可理解的失败提示（不泄露 token），并引导重试/手动录入

### 需求: 管理后台可用性与提示
**模块:** admin
降低“发起授权后卡住/回调失败不知如何处理”的概率。

#### 场景: 远程访问管理后台（localhost 回调限制）
用户通过远程访问管理后台（浏览器在本机，服务在远端）。
- 预期结果: 页面提示需要 `ssh -L 1455:127.0.0.1:1455 ...` 端口转发
- 预期结果: （可选）提供粘贴回调 URL 的兜底流程

## 风险评估
- **风险:** OpenAI `id_token` claims 结构变化导致解析失效
  - **缓解:** 解析逻辑做“主路径 + 兼容兜底”；失败时返回用户可操作提示
- **风险:** 新增“手动粘贴回调”入口可能被误用
  - **缓解:** 复用 `state` 一次性校验 + 仅 root 可用 + 不记录敏感信息

