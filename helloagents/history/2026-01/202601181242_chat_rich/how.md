# 技术设计: Chat 页面增强（上传 / Markdown / 搜索 / 参数）

## 技术方案

### 核心技术
- **前端渲染:** `marked`（Markdown）+ `DOMPurify`（XSS 净化）
- **代码高亮:** `highlight.js`（并提供复制按钮）
- **文件处理:** 浏览器 File API（图片压缩/降采样；文本读取与截断）
- **联网搜索:** 服务端代理到可配置搜索引擎（推荐 `SearXNG` JSON API）

### 实现要点
- 在 `chat.html` 中引入所需的前端库（CDN），并将消息展示从 `textContent` 改为“Markdown → sanitize → innerHTML”。
- 为代码块自动添加复制按钮；复制成功/失败提供轻量提示。
- 增加“附件选择”UI：
  - 图片：限制 MIME（如 `image/png|jpeg|webp|gif`），通过 Canvas 压缩后注入请求（避免超限）。
  - 文本文件：按 MIME/后缀判定可读；读取并按最大字节截断；以文本形式注入消息。
  - 非文本文件：仅注入元信息（文件名/类型/大小）或直接提示不支持。
- 增加“会话设置”：
  - `system_prompt`（可选）
  - `temperature`、`top_p`、`max_output_tokens` 等（按上游兼容保守选择字段；不发送未知字段）
  - `web_search_enabled`（开启时先请求搜索 API，并把结果注入上下文）
- localStorage 结构升级需保持向后兼容：老版本仅包含字符串消息时自动迁移。

## API 设计

### [POST] /api/chat/search
- **描述:** 以 API 请求形式执行联网搜索，返回结构化结果供 `/chat` 注入上下文。
- **认证:** Cookie 会话 + CSRF（`X-CSRF-Token` 或表单 `_csrf`）。
- **请求:** `application/json`
  - `query`（string，必填，长度限制）
  - `limit`（number，可选，默认/上限由服务端控制）
- **响应:** `application/json`
  - `results[]`: `title` / `url` / `snippet`

## 数据模型
无（不引入服务端持久化）

## 安全与性能
- **XSS:** Markdown 输出必须经过净化；限制危险协议；图片预览限制为非 SVG。
- **请求体大小:** 对图片进行压缩/降采样；文本截断；必要时在 UI 提示“超出限制无法发送”。
- **SSRF/滥用:** 搜索仅访问配置的搜索上游（固定 base_url，不允许客户端指定 URL）；请求超时；结果数上限；强制 CSRF。

## 测试与部署
- **测试:** `go test ./...`；对搜索客户端/handler 添加单元测试（含超时与错误分支）。
- **部署:** 在 `config.yaml` 增加搜索配置（如 `search.searxng.base_url`）；未配置时前端自动禁用“联网搜索”入口或提示未启用。

