# upstream

## 职责

- 封装对上游的 HTTP 调用：构造目标 URL、注入鉴权、控制超时与禁止重定向
- 提供 SSE 流式转发工具（大行 buffer、ping 保活、idle 超时与安全的并发写入）

## 接口定义（可选）

### 公共API
| 函数/方法 | 参数 | 返回值 | 说明 |
|----------|------|--------|------|
| `NewExecutor` | `store, cfg` | `*Executor` | 创建上游执行器 |
| `(*Executor).Do` | `ctx, sel, downstreamReq, body` | `*http.Response, error` | 向上游发起请求 |
| `PumpSSE` | `ctx, w, upstreamBody, opts, hooks` | `SSEPumpResult, error` | 透传 SSE（上游→下游） |

## 行为规范

### 场景: base_url 校验
**条件**: 每次构造上游请求  
**行为**: 使用 `security.ValidateBaseURL` 做最小校验（协议/host/DNS）  
**结果**: 降低明显配置错误与 SSRF 风险

### 场景: 上游并发连接数不做按-host限制
**条件**: 数据面存在并发请求（含 SSE/长连接）  
**行为**: 上游 `http.Transport` 不配置 `MaxConnsPerHost`（即不启用按 host 的并发连接上限）  
**结果**: 满足“单 upstream 可多连接”的业务预期，避免因连接上限导致排队/超时后被误判为上游不可用

### 场景: 流式请求不使用 request-level timeout
**条件**: `stream=true` 或 SSE 响应  
**行为**: request-level timeout 由上层控制（避免误伤 SSE 长连接）  
**结果**: SSE 能稳定维持连接

## 依赖关系

```yaml
依赖:
  - security
  - scheduler（Selection 定义）
  - store（读取 credential secret / OAuth token）
被依赖:
  - openai-api
```
