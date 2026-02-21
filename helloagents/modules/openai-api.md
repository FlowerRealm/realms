# openai-api

## 职责

- 实现北向 OpenAI 兼容接口（`/v1/responses`、`/v1/chat/completions`、`/v1/messages`、`/v1/models` 等）
- 负责上游选择（通过 `scheduler`）、失败重试与 failover
- 负责 SSE 流式透传边界（通过 `upstream.PumpSSE`）
- 记录审计/用量事件（如启用）

## 接口定义（可选）

### 公共API
| 函数/方法 | 参数 | 返回值 | 说明 |
|----------|------|--------|------|
| `(*Handler).Responses` | `http.ResponseWriter, *http.Request` | - | 处理 `/v1/responses` |
| `(*Handler).ChatCompletions` | `http.ResponseWriter, *http.Request` | - | 处理 `/v1/chat/completions` |
| `(*Handler).Messages` | `http.ResponseWriter, *http.Request` | - | 处理 `/v1/messages` |
| `(*Handler).Models` | `http.ResponseWriter, *http.Request` | - | 处理 `/v1/models` |

## 行为规范

### 场景: 上游选择 + failover
**条件**: Token 已鉴权、并配置了可用渠道组  
**行为**: 使用 `scheduler` 选择上游；失败时按可重试规则 failover  
**结果**: 成功写回响应；或在无可用上游时返回错误

### 场景: SSE 透传
**条件**: 下游请求为 stream 或上游响应为 `text/event-stream`  
**行为**: 使用 `upstream.PumpSSE` 将上游 SSE 逐行转发到下游  
**结果**: 下游收到 SSE 数据流，连接维持直至结束/断开

### 场景: 上游不可用（最终失败）
**条件**: 已产生至少一次上游 selection，但所有重试/failover 均失败  
**行为**: 返回 `502 上游不可用`；审计/用量事件尽量记录最后一次尝试的上游信息（channel/endpoint/credential）用于排查  
**结果**: 管理侧可在请求明细中定位“最后尝试的渠道”

## 依赖关系

```yaml
依赖:
  - scheduler
  - upstream
  - store（审计/用量/对象引用等）
被依赖:
  - router/openai_routes.go
```
