# 技术设计: 模型定价（input/output/cache）与移除 legacy 上游字段

## 技术方案

### 核心技术
- Go（store/admin/api/quota）
- MySQL migrations（embed SQL）

### 实现要点

1. **数据模型与迁移**
   - 新增迁移 `0011_managed_models_pricing.sql`：
     - 从 `managed_models` 删除 `upstream_type`、`upstream_channel_id` 及其索引。
     - 新增三列：
       - `input_usd_micros_per_1m BIGINT NOT NULL DEFAULT 5000000`
       - `output_usd_micros_per_1m BIGINT NOT NULL DEFAULT 15000000`
       - `cache_usd_micros_per_1m BIGINT NOT NULL DEFAULT 5000000`
   - 默认值选择“偏保守”策略：避免未配置时成本为 0 造成配额失效。

2. **Store 层结构与查询**
   - `store.ManagedModel` 增加 `InputUSDPer1M/OutputUSDPer1M/CacheUSDPer1M`，移除 `UpstreamType/UpstreamChannelID`。
   - `managed_models` 相关 SQL SELECT/INSERT/UPDATE 同步列变更。

3. **管理后台**
   - `/admin/models` 的创建/编辑表单增加三类定价输入（必填）。
   - 提示单位：`usd_micros/1M tokens`（1 USD = 1,000,000 usd_micros）。

4. **数据面路由**
   - `openai.Handler.proxyJSON` 不再回退读取 `managed_models` 的 legacy 上游字段。
   - 统一以 `channel_models` 绑定构造 `AllowChannelIDs` 并进行 `model` alias 重写。
   - 特殊提示保持：
     - 若存在绑定但 chat 无可用 `openai_compatible`，返回“仅支持 /v1/responses”。

5. **计费（成本换算）**
   - 成本计算优先读取 `managed_models` 的三类单价。
   - 若模型不存在（理论上不会发生，但为鲁棒性保留），回退到 `pricing_models` 的 pattern 定价，并将 cache 单价按 input 单价兜底。
   - 结合 `cached_input_tokens/cached_output_tokens`：
     - 非缓存 input tokens 按 input 单价
     - 非缓存 output tokens 按 output 单价
     - 缓存 tokens（cached in/out 汇总）按 cache 单价

## 数据模型

```sql
ALTER TABLE managed_models
  DROP COLUMN upstream_type,
  DROP COLUMN upstream_channel_id,
  ADD COLUMN input_usd_micros_per_1m BIGINT NOT NULL DEFAULT 5000000,
  ADD COLUMN output_usd_micros_per_1m BIGINT NOT NULL DEFAULT 15000000,
  ADD COLUMN cache_usd_micros_per_1m BIGINT NOT NULL DEFAULT 5000000;
```

## 安全与性能

- **安全:** 价格字段输入做数字校验（非数字拒绝）；不允许负数。
- **性能:** 计费侧按 public_id 读取单条 `managed_models`，开销可忽略。

## 测试与部署

- **测试:** 运行 `go test ./...`。
- **部署:** 发布后首次启动会自动执行内置迁移；管理后台需补齐各模型定价（已有默认值可作为兜底）。
