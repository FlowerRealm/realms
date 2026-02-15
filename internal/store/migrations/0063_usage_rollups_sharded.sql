-- 0063_usage_rollups_sharded.sql: 为 hour rollup 增加分片表，降低高并发 finalize 的单行热点。
--
-- 说明：
-- - 仅分片 global_hour 与 channel_hour（热点最大）。
-- - shard 由应用侧计算（usage_event_id % N），通过 env 控制是否启用。
-- - 查询侧对 sharded 表做 SUM 聚合即可得到与旧表一致的结果。

CREATE TABLE IF NOT EXISTS usage_rollup_global_hour_sharded (
  bucket_start DATETIME NOT NULL,
  shard INT NOT NULL,
  requests_total BIGINT NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens BIGINT NOT NULL DEFAULT 0,
  cached_input_tokens BIGINT NOT NULL DEFAULT 0,
  output_tokens BIGINT NOT NULL DEFAULT 0,
  cached_output_tokens BIGINT NOT NULL DEFAULT 0,
  first_token_latency_ms_sum BIGINT NOT NULL DEFAULT 0,
  first_token_samples BIGINT NOT NULL DEFAULT 0,
  decode_latency_ms_sum BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (bucket_start, shard)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS usage_rollup_channel_hour_sharded (
  bucket_start DATETIME NOT NULL,
  upstream_channel_id BIGINT NOT NULL,
  shard INT NOT NULL,
  requests_total BIGINT NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens BIGINT NOT NULL DEFAULT 0,
  cached_input_tokens BIGINT NOT NULL DEFAULT 0,
  output_tokens BIGINT NOT NULL DEFAULT 0,
  cached_output_tokens BIGINT NOT NULL DEFAULT 0,
  first_token_latency_ms_sum BIGINT NOT NULL DEFAULT 0,
  first_token_samples BIGINT NOT NULL DEFAULT 0,
  decode_latency_ms_sum BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (bucket_start, upstream_channel_id, shard),
  KEY idx_usage_rollup_channel_hour_sharded_channel (upstream_channel_id, bucket_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

