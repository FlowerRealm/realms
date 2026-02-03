# 任务清单：本地缓存 tokens 统计

> 目标：在不依赖上游 `usage` 的前提下，统计并计费 `cached_input_tokens`。

## [ ] 实现

- [ ] 在 `internal/tokenizer/` 增加 `EncodeOrdinary`（返回 token ids）能力，用于 LCP 计算。
- [ ] 新增 `internal/tokenizer/promptcache`（或 `internal/billing/promptcache`）模块：
  - [ ] in-memory TTL + LRU（可配置：容量、TTL、max_tokens、block_size）
  - [ ] API：`Get(key) -> ids`, `Put(key, ids, ttl)`
- [ ] 在 OpenAI/Claude/Gemini 入口的 Reserve 阶段：
  - [ ] 解析 `prompt_cache_key`（或复用 route_key）
  - [ ] 生成 cache key（含 token_id + model）
  - [ ] 计算 `cached_input_tokens`（LCP + block 对齐）
  - [ ] 写入/刷新 prompt cache
- [ ] 在 Commit 阶段：
  - [ ] 将 `cached_input_tokens` 写入 `quota.CommitInput.CachedInputTokens`
  - [ ] 落库到 `usage_events.cached_input_tokens`

## [ ] 测试

- [ ] 新增单测：同 `prompt_cache_key` 连续两次请求，第二次 `cached_input_tokens > 0`。
- [ ] 单测覆盖块对齐：LCP=129 时 cached=128（block=128）。
- [ ] 单测覆盖 TTL：过期后 cached=0。

## [ ] 配置与文档

- [ ] 增加配置项（默认值合理）：
  - [ ] `billing.local_prompt_cache_enable`
  - [ ] `billing.local_prompt_cache_ttl`
  - [ ] `billing.local_prompt_cache_capacity`
  - [ ] `billing.local_prompt_cache_block_size`
  - [ ] `billing.local_prompt_cache_max_tokens`
- [ ] 更新 `helloagents/CHANGELOG.md`：记录“本地缓存 tokens 统计方案与实现”。

## [ ] 可选增强（后续）

- [ ] Redis 共享缓存后端（多实例）。
- [ ] 更精确的结构化 token 计算（messages/tools/attachments）。

