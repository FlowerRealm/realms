# 技术设计: Codex OAuth 会话粘性绑定与 RPM 负载均衡

> 说明：本方案包已合并到 `helloagents/history/2026-01/202601131914_codex/`，此处仅保留归档副本（未执行）。

## 技术方案

### 核心技术
- **语言/服务形态:** 复用项目既定的 `codex` Go 服务规划（`net/http` 优先）
- **会话绑定存储:** 内存 TTL（不要求跨重启持久化）
- **负载口径:** rolling RPM（requests per minute）

### 关键设计要点（对齐参考实现）
- 参考 `rc-balance`：以 routeKey 做粘性（pin）并在不可用时 failover，同时用并发/冷却保护；本方案采用其“routeKey→账号绑定 + 失败重绑”的核心思路，但把负载口径改为 **RPM**。
- 参考 `claude-proxy`：failover 必须有上限与 failed-set，避免死循环；并明确“流式写回后禁止 failover”的边界。

## 架构设计（增量）

```mermaid
flowchart TD
  In[HTTP Request] --> RK[Extract routeKey]
  RK -->|hit| Pin[Pin: routeKey->channel/account]
  RK -->|miss| Pick[Pick by RPM (available only)]
  Pin --> Try[Try bound channel]
  Pick --> Bind[Bind routeKey TTL=30m]
  Bind --> Try
  Try -->|ok| Done[Return response + touch TTL]
  Try -->|fail x3| Rebind[Rebind to new lowest-RPM candidate]
  Rebind --> Try2[Try new channel (bounded)]
  Try2 --> Done
```

## 关键逻辑定义

### 1) routeKey 提取规则（固定）
优先级（从高到低）：
1. 请求体 JSON 顶层字段：`prompt_cache_key`
2. 请求头：`Conversation_id`
3. 请求头：`Session_id`
4. 请求头：`Idempotency-Key`

规范化：
- trim 空白；空字符串视为不存在
- 内部存储建议用 `sha256(routeKey)`（避免长 key/敏感值直接进入内存结构与日志字段）

### 2) 会话绑定（TTL=30min，临时存储）
数据结构（内存）：
- `binding[routeKeyHash] = { accountId, expiresAt }`
- 每次成功命中后 `touch`（expiresAt = now + 30min）
- 定期清理过期项（例如每 1-5 分钟一次；O(n) 足够，账号数量通常较小）

> 用户明确要求“不需要跨重启”，因此不引入 MySQL/Redis 的粘性持久化。

### 3) RPM 口径与实现
目标：用于“新会话绑定”与“重绑后选号”。

建议实现：
- 对每个 `accountId` 维护 rolling window 计数（窗口 60s）
- 每次 **上游尝试（包含重试）** 计入一次（反映真实压力）
- `rpm(account) = sum(windowBuckets)`；选择 rpm 最小者
- tie-break：按稳定顺序（配置顺序或 accountId 字典序），保证行为可预期

### 4) 重试→重绑（所有错误都重试）
单次请求的调度策略（当 routeKey 存在）：
1. 若已绑定且绑定账号可用：先在该账号上尝试；**失败后重试直到累计 3 次尝试**。
2. 仍失败：选择其他可用账号中 RPM 最低者，更新绑定并继续尝试。
3. 为避免死循环：对“账号切换次数”设置上限（≤候选可用账号数），并维护 `excludedAccountIds`。

流式边界（强约束）：
- 仅在未向下游写回任何响应（header/body/首字节）前允许重试/重绑
- 一旦开始写回，本次请求固定账号；错误直接向下游返回，不再切换

## 配置建议（最小）

```yaml
codex_oauth:
  session_ttl: 30m
  retry_attempts: 3
  rpm_window: 60s
```

> 如后续出现“失败账号被立即再次选中”的抖动，可再加一个可选项 `cooldown_on_fail`（默认 0 关闭），但不作为本次必需项。

## 安全与合规
- routeKey 仅用于路由；默认不写日志原值（仅 hash 或完全不记录）
- 严格限制重试次数与总尝试上限，避免放大故障
- SSE 写回后禁止 failover，避免重复输出/计费语义风险

## 测试策略（建议）
- 单测：routeKey 提取优先级、TTL 续期与过期、RPM 选择稳定性、重试 3 次后才重绑
- 集成/端到端：模拟上游连续失败触发重绑；模拟 SSE 首包前失败允许重试、首包后失败不切换
