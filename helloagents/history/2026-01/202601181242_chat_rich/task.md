# 任务清单: Chat 页面增强（上传 / Markdown / 搜索 / 参数）

目录: `helloagents/plan/202601181242_chat_rich/`

---

## 1. 联网搜索（API 请求）
- [√] 1.1 在 `internal/config/config.go` 与 `config.example.yaml` 中加入搜索配置（SearXNG base_url/timeout/limit），验证 why.md#需求-联网搜索（api-请求）-场景-开启联网搜索并提问
- [√] 1.2 新增 `internal/search`（SearXNG client）并在 `internal/web/chat.go` 中实现 `POST /api/chat/search`（Cookie+CSRF），验证 why.md#需求-联网搜索（api-请求）-场景-开启联网搜索并提问
- [√] 1.3 在 `internal/server/app.go` 注册路由并打通页面调用，验证 why.md#需求-联网搜索（api-请求）-场景-开启联网搜索并提问

## 2. Markdown 渲染 + 代码高亮复制
- [√] 2.1 在 `internal/web/templates/chat.html` 中接入 Markdown 渲染（marked+DOMPurify），并保持纯文本回退，验证 why.md#需求-markdown-渲染--代码高亮复制-场景-阅读与复制代码块
- [√] 2.2 在 `internal/web/templates/chat.html` 中接入代码高亮与代码块复制按钮，验证 why.md#需求-markdown-渲染--代码高亮复制-场景-阅读与复制代码块

## 3. 图片/文件上传
- [√] 3.1 在 `internal/web/templates/chat.html` 中加入附件选择 UI（图片+文本文件），并将内容注入发送消息（含大小限制与提示），验证 why.md#需求-图片文件上传-场景-上传图片并提问
- [√] 3.2 在 `internal/web/templates/chat.html` 中实现文本文件读取/截断/展示与非文本文件提示，验证 why.md#需求-图片文件上传-场景-上传文本类文件并提问

## 4. 对话参数可调
- [√] 4.1 在 `internal/web/templates/chat.html` 中加入会话级参数面板（system prompt/temperature/top_p/max_output_tokens），并持久化到 localStorage，验证 why.md#需求-对话参数可调-场景-调整温度与输出长度
- [√] 4.2 在 `internal/web/templates/chat.html` 中将参数注入 `/v1/responses` payload（仅发送兼容字段），验证 why.md#需求-对话参数可调-场景-调整温度与输出长度

## 5. 安全检查
- [√] 5.1 执行安全检查（按G9: XSS/SSRF/输入校验/敏感信息提示/附件大小限制）

## 6. 文档更新
- [√] 6.1 更新 `helloagents/wiki/api.md`：补充 `/api/chat/search` 与 `/chat` 新能力说明
- [√] 6.2 更新 `helloagents/CHANGELOG.md`：记录本次变更（Unreleased）

## 7. 测试
- [√] 7.1 运行 `go test ./...` 并修复本次改动引入的问题
