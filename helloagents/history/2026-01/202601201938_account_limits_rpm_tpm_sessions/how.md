# 技术设计: 按账号限额（RPM/TPM/会话数）

## 技术方案

### 核心目标
- 限额维度从 channel 调整为 **account（credential/account）**
- 限额字段收敛为：`limit_sessions / limit_rpm / limit_tpm`
- 超限处理：**跳过该账号并 failover**（不阻断整次请求）

### 实现要点

#### 1) 数据模型与迁移

新增迁移（建议编号：0035）：
- `upstream_channels`：将 `limit_cc` 重命名为 `limit_sessions`（仅为语义对齐与历史兼容）
- `openai_compatible_credentials`：增加 `limit_sessions/limit_rpm/limit_tpm`
- `codex_oauth_accounts`：增加 `limit_sessions/limit_rpm/limit_tpm`
- 回填策略（可选但推荐）：当账号未显式设置 limits 时，用所属 channel 的 limits 作为默认值回填

取值约定：
- `NULL` = 不限制
- `<=0` = 视为未设置（按 `NULL` 处理）

#### 2) Store 层（读写 limits）

- 扩展 `store.OpenAICompatibleCredential` / `store.CodexOAuthAccount` 结构体字段
- `ListOpenAICompatibleCredentialsByEndpoint` / `ListCodexOAuthAccountsByEndpoint` 查询增加 limits 列
- 增加更新接口：
  - `UpdateOpenAICompatibleCredentialLimits(ctx, id, limitSessions, limitRPM, limitTPM)`
  - `UpdateCodexOAuthAccountLimits(ctx, id, limitSessions, limitRPM, limitTPM)`

#### 3) Scheduler：按账号限额过滤 + sessions 计数

**sessions 计数口径（关键）**
- 复用现有 session 粘性绑定机制：`(user_id, route_key_hash) -> Selection(credential_key)`
- sessions = 某个 `credential_key` 当前被多少个 binding 占用（未过期）
- binding 的 TTL 沿用现有 `bindingTTL`（默认 30min），到期自动释放 sessions

**调度过滤策略**
- 当命中既有 binding：直接复用（不受新会话 sessions 限制影响）
- 当需要为新会话选择账号时：
  - 过滤掉已冷却账号
  - 过滤掉 `sessions/rpm/tpm` 任一超限的账号
  - 在剩余账号中按既有策略排序（优先低 RPM、结合失败分）

**状态数据结构**
- `State` 增加：
  - 绑定到账号的 session 计数（支持过期清理）
  - 账号维度的 token 事件滑动窗口（TPM 计算）
- 对 sessions 过期清理采取“节流扫尾”策略：避免每次请求全量扫描

#### 4) TPM 统计回流

TPM 需要在请求完成后得到 usage 才能累计：
- 非流式：从响应 JSON 的 `usage` 提取 input/output（已有逻辑）
- 流式：仅在事件中包含 usage 时累计（已有解析逻辑）

在 `handler` 成功拿到 usage 后，将 `(input+output+cached)` 的总 tokens 记录到 `scheduler.State`：
- 记录维度：`sel.CredentialKey()`
- 记录时机：请求完成（commit/最终化）时

限制说明：
- 无 usage 时无法精确统计 TPM，该请求不计入 TPM（文档明确此点）

#### 5) 管理后台：按账号编辑 limits

UI 放置建议：
- `端点与授权管理` 页面（`endpoints.html`）中：
  - OpenAI credentials 列表：每行展示 `sessions/rpm/tpm`，提供“编辑限额”按钮/表单
  - Codex OAuth accounts 列表：同样增加 limits 展示与编辑

API 设计（管理面）：
- `POST /admin/openai-credentials/{id}/limits`
- `POST /admin/codex-accounts/{id}/limits`
- 权限：root-only

## 安全与性能

- **安全**
  - limits 修改为 root-only，且所有输入做整数解析与范围校验
  - 不新增敏感信息落盘

- **性能**
  - sessions 计数与过期清理：使用节流扫尾避免每次全量扫描 binding map
  - TPM 统计滑动窗口：仅保留窗口内事件，避免无界增长

## 测试与部署

- **单测**
  - Scheduler：sessions/rpm/tpm 超限过滤、binding 复用与过期释放
  - Store：limits 读写（可通过已有 store 测试模式或最小集成测试覆盖）
- **回归**
  - `go test ./...`
- **部署**
  - 运行迁移后，管理后台在端点页配置 limits 即可生效

