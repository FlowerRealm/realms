-- 0061_usage_rollups.sql: 为百万级 usage_events 规模引入 rollup 表 + 幂等标记。
--
-- 说明：
-- - rollup_applied_at 用于保证“每个 usage_event 只汇总一次”，避免 finalize 重入/补算时重复计数。
-- - 目前 rollup 以 hour/day 为粒度，覆盖全站/按渠道/按用户/按模型的核心统计查询。

ALTER TABLE usage_events
  ADD COLUMN rollup_applied_at DATETIME NULL;

CREATE INDEX idx_usage_events_rollup_applied_at ON usage_events(rollup_applied_at);

CREATE TABLE IF NOT EXISTS usage_rollup_global_hour (
  bucket_start DATETIME NOT NULL,
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
  PRIMARY KEY (bucket_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS usage_rollup_channel_hour (
  bucket_start DATETIME NOT NULL,
  upstream_channel_id BIGINT NOT NULL,
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
  PRIMARY KEY (bucket_start, upstream_channel_id),
  KEY idx_usage_rollup_channel_hour_channel (upstream_channel_id, bucket_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS usage_rollup_user_day (
  day DATE NOT NULL,
  user_id BIGINT NOT NULL,
  requests_total BIGINT NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens BIGINT NOT NULL DEFAULT 0,
  cached_input_tokens BIGINT NOT NULL DEFAULT 0,
  output_tokens BIGINT NOT NULL DEFAULT 0,
  cached_output_tokens BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (day, user_id),
  KEY idx_usage_rollup_user_day_user (user_id, day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS usage_rollup_model_day (
  day DATE NOT NULL,
  model VARCHAR(128) NOT NULL,
  requests_total BIGINT NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens BIGINT NOT NULL DEFAULT 0,
  cached_input_tokens BIGINT NOT NULL DEFAULT 0,
  output_tokens BIGINT NOT NULL DEFAULT 0,
  cached_output_tokens BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (day, model),
  KEY idx_usage_rollup_model_day_model (model, day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

