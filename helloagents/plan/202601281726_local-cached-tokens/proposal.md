# 本地 Token 计费 + 缓存 Token 统计方案（不依赖上游 usage）

## 背景

当前 Realms 已实现“本地计算 input/output tokens 并用于计费”，不再依赖上游返回的 `usage` 字段。
但 **缓存 tokens（如 OpenAI prompt caching 的 `cached_tokens`）无法仅从请求/响应内容反推出**：它本质是“缓存命中”信号，需要服务端维护缓存状态。

你提到可参考 `new-api` 仓库。经对比，`new-api` 的缓存计费主要做两件事：

1. **从上游返回的 usage 中读取缓存 tokens**（例如 `usage.prompt_tokens_details.cached_tokens`，以及部分渠道的非标准位置兜底提取）。
2. 按 `cache_ratio` 等倍率将缓存 tokens 计入计费与日志统计。

这意味着：如果我们坚持“完全本地计算、不依赖上游 usage”，就必须 **在 Realms 自己实现 prompt cache 的命中与统计**。

## 目标（需求）

1. **完全本地计算**：不读取/不解析上游响应的 `usage` 来决定计费 tokens。
2. **统计缓存**：落库 `usage_events.cached_input_tokens`（必要）与 `cached_output_tokens`（可选，默认 0）。
3. **计费口径可落地**：继续使用 `managed_models.cache_input_usd_per_1m/cache_output_usd_per_1m` 的价格字段。
4. **性能可控**：避免为每次请求保存整段文本或无限增长；有上限与 TTL。
5. **可扩展**：单机默认 in-memory；需要多实例一致性时可选共享后端（Redis/DB）。

## 关键难点

缓存 tokens 的“命中数量”不是纯函数：  
同样的请求内容，在“缓存未建立 / 已建立 / 已过期”的状态下，缓存 tokens 会不同。

因此，要做到本地统计，必须引入 **服务端 prompt cache 状态机**。

## 推荐方案：本地 Prompt Cache（基于 prompt_cache_key 的前缀命中）

### 1) Cache Key 设计

复用 Realms 已有的 route key 思路（header/payload 的 `prompt_cache_key` 等来源），组成一个稳定 key：

```
cache_key = hash(token_id, model, prompt_cache_key_or_route_key)
```

说明：
- `token_id`：避免不同用户互相污染缓存。
- `model`：不同模型编码不同，缓存不可复用。
- `prompt_cache_key`：由客户端提供，用于显式控制缓存命中。

### 2) 缓存内容

为了计算“命中 tokens 数”，最直接的方式是缓存 **prompt 的 token id 序列**（不是仅缓存总数）：

- `prompt_token_ids []int`（只保留前 N 个 tokens，用于前缀比较）
- `expires_at time.Time`

为什么要存 ids：  
缓存命中通常表现为“请求 prompt 的最长公共前缀（LCP）被复用”。只存总数无法算 LCP。

### 3) 命中计算（cached_input_tokens）

对同 key 的上一次 prompt 与当前 prompt 做 LCP：

```
cached = longest_common_prefix_len(prev_ids, curr_ids)
```

为了更贴近实际 prompt caching，可选加入两个规则：

- **块对齐**：`cached = floor(cached / 128) * 128`（默认 128，可配置）
- **最小门槛**：如果 cached < block_size，则 cached = 0（可配置）

### 4) 何时写缓存

在 **收到请求并完成本地 token 化之后** 即写入（或更新）缓存：

- 第一次请求：cached_input_tokens=0（建立缓存）
- 后续请求：先读旧缓存算 cached，再写入新 token_ids 刷新 TTL

### 5) cached_output_tokens

默认不做（置 0 / NULL），原因：
- 主流“提示词缓存”只影响输入侧。
- 输出缓存需要实现“响应缓存”，会引入更高风险（一致性/隐私/幂等）。

如确需 output 缓存，建议单独开一个“响应缓存（response cache）”方案，不与 prompt cache 混做。

### 6) 计费落库与展示

在 `quota.Commit` 时写入：
- `input_tokens`（本地算）
- `cached_input_tokens`（本地 prompt cache 算）
- `output_tokens`（本地算）
- `cached_output_tokens`（默认 0）

计费使用现有逻辑：
- 非缓存 tokens 按 `input_usd_per_1m/output_usd_per_1m`
- 缓存 tokens 按 `cache_input_usd_per_1m/cache_output_usd_per_1m`

### 7) 存储后端（两档）

**A. 单机默认：In-memory TTL + LRU（推荐起步）**
- 优点：实现简单，性能最好。
- 缺点：多实例不共享；重启丢失。

**B. 多实例：Redis（推荐）或 DB 表（备选）**
- Redis：key-value 存 `[]int`（压缩/二进制），TTL 原生支持。
- DB：建表 `prompt_cache_entries`（key、token_ids_blob、expires_at、updated_at），实现更重但依赖更少。

## 方案边界与风险说明

1. **与上游真实 cached_tokens 可能不一致**：上游可能有额外规则（块大小、最小阈值、内部 prompt 序列化差异）。
2. **输入提取方式会影响缓存命中**：如果“输入文本提取”把 JSON 结构串成字符串，LCP 可能不稳定。建议尽量做结构化提取（messages/content/input）。
3. **多实例一致性**：如果你跑多副本，需要共享后端，否则同 key 在不同实例上会出现“命中为 0”的抖动。

## 建议的落地顺序

1. 先实现 **In-memory Prompt Cache + cached_input_tokens**（含块对齐、TTL、最大 tokens）。
2. 再补 **Redis 共享**（如果你确实是多实例部署）。
3. 最后再考虑“更精确的 Chat/Responses token 口径”（包括 role/工具调用等结构化计数）。

