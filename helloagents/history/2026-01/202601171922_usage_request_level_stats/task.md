# 任务清单: 用量统计请求级明细（每次请求可追溯）

目录: `helloagents/plan/202601171922_usage_request_level_stats/`

---

## 1. 数据模型（usage_events）
- [√] 1.1 增加请求级明细字段：`endpoint/status_code/latency_ms/error_class/error_message/is_stream/request_bytes/response_bytes`
- [√] 1.2 store 增加 `FinalizeUsageEvent` 与按区间查询（用户/全站）

## 2. 数据面采集（OpenAI 代理链路）
- [√] 2.1 在成功/失败/流式路径落库请求明细（按 request_id 对齐）
- [√] 2.2 commit/void 使用后台短超时 context，避免客户端断开导致用量停留 reserved

## 3. 展示与 API
- [√] 3.1 用户 `/usage` 增加“请求明细”表（支持区间与分页）
- [√] 3.2 管理后台 `/admin/usage` 增加“请求明细”表（全站视角，展示用户信息）
- [√] 3.3 `/api/usage/events` 补齐请求明细字段，并支持可选 `start/end` 区间过滤

## 4. 验证与文档
- [√] 4.1 运行 `go test ./...`
- [√] 4.2 同步更新知识库：`helloagents/wiki/data.md`、`helloagents/wiki/api.md`、`helloagents/wiki/modules/realms.md`、`helloagents/CHANGELOG.md`

