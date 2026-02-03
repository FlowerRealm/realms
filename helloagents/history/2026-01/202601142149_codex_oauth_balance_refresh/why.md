# 变更提案: Codex OAuth 账号额度自动刷新（后台）

## 需求背景
当前管理后台仅能展示 Codex OAuth 账号的订阅状态（来自 `id_token` claims），但无法看到“剩余额度/可用额度”信息。对于多账号场景，管理员需要快速识别哪些账号额度充足/不足，以及拉取失败的原因，便于运维与账号池管理。

## 变更内容
1. 为 Codex OAuth Accounts 增加“剩余额度（USD）/更新时间/错误”展示字段。
2. 服务端后台定时刷新所有 Codex OAuth 账号的额度信息（每 10 分钟一次）。
3. 刷新失败时记录并在后台直接展示错误信息（不做静默吞错）。

## 影响范围
- **模块:** `internal/codexoauth`、`internal/server`、`internal/store`、`internal/admin`
- **文件:** 见 task.md
- **API:** 无（仅管理后台展示增强）
- **数据:** `codex_oauth_accounts` 表增加额度相关字段

## 核心场景

### 需求: 后台展示 Codex OAuth 账号剩余额度
**模块:** admin/store
管理员在 `/admin/channels/{id}/endpoints#accounts` 的账号列表中，能看到每个账号的剩余额度（USD）、最近刷新时间，以及失败时的错误信息。

#### 场景: 查看账号余额
打开 `codex_oauth` 渠道的 Endpoints 页面。
- 预期结果：每个账号行展示 `Available/Used/Granted`（或至少 Available），并显示 `UpdatedAt`；若失败则显示错误字符串。

### 需求: 定时刷新所有账号额度
**模块:** server/codexoauth/store
服务启动后，后台每 10 分钟刷新一次所有 Codex OAuth 账号额度信息并落库。

#### 场景: 自动刷新
服务运行期间无需人工触发。
- 预期结果：账号额度字段会随时间更新；刷新失败时写入错误信息并更新时间。

## 风险评估
- **风险:** 上游额度接口不稳定/变更，可能返回非预期结构或重定向到登录页，导致解析失败。
- **缓解:** 将失败视为“可观测错误”落库并展示；解析与错误信息做截断，避免污染页面与日志；不影响数据面请求转发。

