# 技术设计: Codex OAuth 账号额度自动刷新（后台）

## 技术方案

### 核心技术
- Go `net/http` 定时任务（goroutine + ticker）
- MySQL schema 迁移（新增列）
- 复用现有 Codex OAuth refresh 逻辑（`refresh_token` → `access_token`）

### 实现要点
- **数据落库：**在 `codex_oauth_accounts` 表新增额度字段：
  - `balance_total_granted_usd_micros`
  - `balance_total_used_usd_micros`
  - `balance_total_available_usd_micros`
  - `balance_updated_at`
  - `balance_error`
- **刷新频率：**10 分钟一次（服务端后台任务），启动后立即跑一轮（便于尽快有数据）。
- **上游拉取：**用 OAuth `access_token` 携带 `Chatgpt-Account-Id`，请求 credit_grants 类接口；解析 `total_available/total_used/total_granted` 并换算为 `usd_micros`。
- **失败策略：**失败时写入 `balance_error` 与 `balance_updated_at`，页面直接显示错误；成功时清空 `balance_error`。
- **Token 刷新：**
  - 若 `expires_at` 临近（<5min）则先 refresh 再拉取额度。
  - 若拉取额度返回未授权类错误，再尝试 refresh 并重试一次。

## 安全与性能
- **安全:**
  - 错误信息截断（≤255）避免把大段 HTML/JSON 注入页面。
  - 不在日志中输出 token。
- **性能:**
  - 单轮刷新默认串行；账号数量通常较少（管理面）。如未来账号规模变大，再考虑并发与限流（YAGNI）。

## 测试与部署
- **测试:**
  - `internal/codexoauth` 增加单测：额度 JSON 解析与非 2xx 错误处理。
  - `go test ./...` 全量跑一遍。
- **部署:**
  - 迁移文件通过内置迁移机制自动执行；无需额外手工步骤。

