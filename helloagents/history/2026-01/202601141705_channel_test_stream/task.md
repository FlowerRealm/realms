# 任务清单（轻量迭代）

目标：管理后台“渠道测试”使用 **流式（SSE）** 测试，真实验证上游 stream 能力，并展示 TTFT/示例输出。

- [√] 测试请求强制 `stream=true`，并设置 `Accept: text/event-stream`
- [√] SSE 解析：读取若干 `data:` 事件，提取增量文本作为示例输出，并计算 TTFT
- [√] 失败策略：上游未返回 `text/event-stream` 视为测试失败（明确报错）
- [√] 管理后台提示：测试成功后在 `/admin/channels` 展示 TTFT 与示例输出
- [√] 测试验证：`go test ./...`
- [√] 迁移方案包至 `helloagents/history/` 并更新索引

