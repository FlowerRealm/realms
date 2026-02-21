# middleware

## 职责

- 提供数据面中间件链：RequestID、AccessLog、TokenAuth、BodyCache、FeatureGate 等
- 在进入业务 handler 之前完成基础鉴权与请求体缓存

## 接口定义（可选）

### 公共API
| 函数/方法 | 参数 | 返回值 | 说明 |
|----------|------|--------|------|
| `RequestID` | `http.Handler` | `http.Handler` | 注入 request_id |
| `AccessLog` | `http.Handler` | `http.Handler` | slog 访问日志 |
| `TokenAuth` | `*store.Store` | `Middleware` | Bearer/x-api-key 鉴权 |
| `BodyCache` | `maxBytes` | `Middleware` | 缓存 request body 供多次读取 |

## 行为规范

### 场景: Token 鉴权
**条件**: 请求包含 `Authorization: Bearer <token>` 或 `x-api-key`  
**行为**: 查询 token 授权信息并注入 Principal  
**结果**: 下游 handler 可从 context 获取用户身份与渠道组

## 依赖关系

```yaml
依赖:
  - store
  - auth
被依赖:
  - router/openai_routes.go
  - router/public_routes.go
```

