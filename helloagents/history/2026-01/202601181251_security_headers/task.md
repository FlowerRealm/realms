# 轻量迭代：安全头处理与 Base URL 推断加固

目标：修复上游转发的敏感/跳跃头透传、限制并校验 `X-Forwarded-*` 参与 Base URL 推断、以及 `request_id` 随机数生成失败的退化处理。

## 任务清单

- [√] [P1] 上游转发：显式剥离 Cookie 与 RFC 7230 hop-by-hop 头（含 Connection 指定的附加头）；补齐单测
- [√] [P2] Base URL：仅在请求来自 `trusted_proxy_cidrs` 时信任 `X-Forwarded-*`，并校验 proto/host；同步修正文档与示例配置；补齐单测
- [√] [P3] request_id：处理 `crypto/rand.Read` 失败场景，避免生成全 0 / 低熵 ID；补齐单测
- [√] 运行 `go test ./...`
