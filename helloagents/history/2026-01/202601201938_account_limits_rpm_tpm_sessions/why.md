# 变更提案: 按账号限额（RPM/TPM/会话数）

## 需求背景

当前 Realms 已有上游限额能力，但存在两类不匹配：

1. **口径不一致**：`cc` 实际是 *concurrency（并发在途请求数）*，而需求要的是 **会话 ID 级别的会话数（sessions）**。
2. **维度不一致**：需求要按 **account** 生效（例如 URL 渠道按每个 API Key；Codex OAuth 按每个账号），而不是按 channel 维度。

因此需要将限额能力收敛为 3 个字段，并在调度时严格按账号维度生效：**rpm / tpm / sessions**。超限时不报错阻断，而是 **跳过当前账号并 failover 到下一个可用账号**。

## 变更内容

1. **数据模型新增（账号维度）**
   - `openai_compatible_credentials` 与 `codex_oauth_accounts` 增加：
     - `limit_sessions`：最大会话数（会话 ID 级别）
     - `limit_rpm`：每分钟请求数
     - `limit_tpm`：每分钟 tokens 总量（input+output）
   - 取值规则：`NULL` 表示无限制；`<=0` 视为未设置（按 `NULL` 处理）。

2. **调度策略变更（账号维度生效）**
   - 调度在选择 credential/account 时同时检查 `sessions/rpm/tpm`：
     - 任一超限则跳过该账号，选择下一个。
   - 保持现有失败冷却/ban 机制：请求失败会降低权重并触发冷却，下一次选择会自然避开。

3. **sessions 的定义**
   - 使用请求中的会话标识（已有 `routeKey` 机制：从 `Conversation-Id/Session-Id/...` 等头或 payload 取值并 hash）。
   - **按“绑定中的 session 数”计数**：一个 `(user_id, route_key_hash)` 绑定到某个账号，即视为该账号占用 1 个 session；随 binding TTL 过期自动释放。

4. **改名与迁移**
   - 将历史字段 `limit_cc` 改名为 `limit_sessions`（语义对齐）。
   - 将 channel 上已配置的默认值回填到账号（仅在账号未显式设置时回填），以支持平滑过渡。

5. **管理面与文档同步**
   - 管理后台在 **端点详情页（keys/accounts 列表）**提供 limits 的查看与编辑入口。
   - 更新知识库：数据模型、管理面 API、模块说明与变更记录。

## 影响范围

- **模块**
  - `internal/store`: 新增/迁移列与读写接口
  - `internal/scheduler`: sessions/rpm/tpm 约束过滤与状态统计
  - `internal/api/openai`: tpm 统计数据回流（从响应 usage 提取）
  - `internal/admin`: 管理面 limits 编辑入口

- **数据**
  - 新增列与一次性回填（不会删除现有数据）

## 核心场景

### 场景 1：按账号 RPM 限额自动切换
- 条件：账号 A `limit_rpm=60`，账号 B 不限
- 预期：A 在 60 次/分钟达到上限后，新请求自动切到 B

### 场景 2：按会话数（sessions）限制新会话绑定
- 条件：账号 A `limit_sessions=2`，当前已有两个不同 `route_key` 绑定到 A
- 预期：第三个新会话不会绑定到 A，而会绑定到下一个可用账号

### 场景 3：按账号 TPM 限额自动切换
- 条件：账号 A `limit_tpm` 较小且在 60s 内累计 tokens 接近上限
- 预期：当累计 tokens 超过上限时，新请求自动切到下一个账号

## 风险评估

- **风险：TPM 统计不完整**
  - 原因：部分上游/请求形态可能不返回 usage，或流式未开启 `include_usage`
  - 缓解：仅在拿到 usage 时计入 TPM；文档明确该限制依赖 usage；建议上游开启 usage 或在必要时启用 `include_usage`

- **风险：缺失 routeKey 时 sessions 不生效**
  - 缓解：沿用现有逻辑：routeKey 为空则不绑定；文档提示客户端需带 `Conversation-Id/Session-Id`（或使用 payload 的 `prompt_cache_key`）以启用 sessions 限制

