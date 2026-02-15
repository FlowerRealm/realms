# MySQL：rollup 分片“锁热点下降”采证流程

目标：为 sharded hour rollup（`internal/store/migrations/0063_usage_rollups_sharded.sql:1`、`internal/store/usage_rollups.go:42`）提供可复现证据，证明高并发 finalize 时锁等待/冲突显著下降。

## 前置条件
- 使用 MySQL（建议 8.x，开启 performance_schema 更好）
- 有稳定的高并发 finalize/请求压测方式（可复用 `scripts/load-curl-responses.sh:1`）

## 实验矩阵（两组对比）

### 组 A：不启用分片
- 不设置 `REALMS_USAGE_ROLLUP_SHARDS` / `REALMS_USAGE_ROLLUP_SHARDS_CUTOVER_AT`（见 `.env.example:60`）

### 组 B：启用分片
- 设置：
  - `REALMS_USAGE_ROLLUP_SHARDS=16`（或 32）
  - `REALMS_USAGE_ROLLUP_SHARDS_CUTOVER_AT=<RFC3339 UTC>`（所有实例一致）

## 压测步骤（每组都做一遍）

1) 启动 Realms（连接到 MySQL）
2) 在压测窗口内跑请求负载（示例）：
```
REALMS_LOAD_REQUESTS=5000 REALMS_LOAD_PARALLEL=200 bash scripts/load-curl-responses.sh
```
3) 同时采集锁等待证据（建议 30–120s）：
```
REALMS_EVIDENCE_SECONDS=60 bash scripts/mysql-capture-lockwaits.sh
```
脚本输出会写到 `output/mysql-lockwaits-*.log`。

## 期望结果（判定）
- 组 B 相比组 A：
  - 与 `usage_rollup_global_hour` / `usage_rollup_global_hour_sharded`、`usage_rollup_channel_hour` / `usage_rollup_channel_hour_sharded` 相关的 lock wait 计数下降
  - 负载更平稳（p95/p99 latency、错误率更低）

备注：性能与锁等待会受 MySQL 参数、磁盘/CPU、连接池、并发模式影响；建议固定环境与压测脚本参数再对比。

