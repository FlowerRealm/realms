# 任务清单: SSE StreamPump（对齐 new-api）与流式 token 统计

目录: `helloagents/plan/202601181616_sse_stream_pump/`

---

## 1. upstream: StreamPump（替换/增强 RelaySSE）
- [√] 1.1 在 `internal/upstream/` 新增 StreamPump 实现（scanner 大 buffer、写互斥、ping、idle-timeout），验证 why.md#核心场景-需求-支持大-sse-事件行
- [√] 1.2 保持 `RelaySSE(ctx,w,body)` 兼容（作为 wrapper 或默认参数调用），并补齐单测（大事件行/flush/idle-timeout/ctx cancel）

## 2. middleware: 流式感知的超时策略
- [√] 2.1 在 `internal/middleware/` 实现“流式感知 timeout”（按 `POST /v1/responses`、`POST /v1/chat/completions` 跳过 deadline），并更新 `internal/server/app.go` 使用，验证 why.md#核心场景-需求-流式不断联（长连接）
- [√] 2.2 补充 middleware 单测：非流式仍受 max_request_duration 限制；流式路径不被 2m 误杀

## 3. upstream executor: 去除 SSE 的硬超时
- [√] 3.1 调整 `internal/upstream/executor.go`：取消全局 `http.Client.Timeout`（或拆分 stream/non-stream client），确保 SSE 不被 `upstream_request_timeout` 硬切断，验证 why.md#需求-流式不断联（长连接）
- [√] 3.2 补充/更新 executor 相关测试（如有）：确认重定向策略与 header timeout 不受影响

## 4. openai handler: 流式 token 统计与错误分类
- [√] 4.1 在 `internal/api/openai/handler.go` 的 SSE 分支接入 StreamPump hook：解析 SSE `data:` JSON 的 usage tokens（含 cached_tokens），并在结束时 `quota.Commit` 写入 tokens，验证 why.md#核心场景-需求-流式-token-统计准确
- [√] 4.2 将 StreamPump 返回的错误映射为稳定 `error_class`，并保证断联时 usage_event 能 finalize（避免 reserved 悬挂），验证 why.md#核心场景-需求-断联可诊断
- [√] 4.3 扩展非流式 `extractUsageTokens` 的兼容路径（如发现 usage 嵌套结构），补齐单测（可复用现有测试框架）

## 5. 配置与文档
- [√] 5.1 在 `internal/config/config.go` 增加 stream 相关配置项（例如 `stream_idle_timeout` / `sse_ping_interval` / `sse_max_event_bytes` / `max_stream_duration`），并更新 `config.example.yaml`（如存在）/ `config.yaml` 示例注释
- [√] 5.2 更新知识库说明（建议：`helloagents/wiki/modules/realms.md` 补充“流式转发与统计口径/配置项”章节）

## 6. 安全检查
- [√] 6.1 执行安全检查（按G9：输入验证、敏感信息处理、权限控制、避免写入请求/响应明文；检查新增配置项不会引入 SSRF/日志泄露）

## 7. 测试
- [√] 7.1 运行 `go test ./...`，并补充针对断联/大行/usage 提取的关键场景测试用例
