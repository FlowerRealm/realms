# 技术设计: 渐进式 AJAX 表单提交（不跳转 / 不污染 URL）

## 技术方案

### 核心技术
- SSR 模板内置轻量 JS（无前端构建链路）
- `fetch` + `FormData`/`URLSearchParams` 提交表单
- 服务端按 header 分流：HTML(PRG) vs JSON(AJAX)

### 实现要点
#### 1) 前端：通用 AJAX 表单拦截器
在 `internal/web/templates/base.html` 与 `internal/admin/templates/base.html` 增加通用脚本：
- 仅拦截带 `data-ajax="1"` 的 `<form>`（渐进式开启）
- 读取隐藏字段 `_csrf`，写入请求头 `X-CSRF-Token`
- 附加请求头：`X-Realms-Ajax: 1`、`Accept: application/json`
- 默认使用 `URLSearchParams(new FormData(form))`；若存在文件上传（`input[type=file]` 或 `enctype=multipart/form-data`）则使用 `FormData`
- 提交期间禁用 submit 按钮，防止重复提交
- 解析 JSON 响应：
  - `{ok:true, notice}` → 展示成功提示
  - `{ok:false, error}` → 展示错误提示
  - 可选：表单加 `data-ajax-reload="1"` 时成功后 `location.reload()`（用于列表页的创建/删除）
- 网络/解析失败：展示通用错误提示

#### 2) 服务端：为高频 POST handler 增加 AJAX 分支
在 `internal/admin` 与 `internal/web` 增加轻量 helper（同包内即可，避免过度抽象）：
- `isAjax(r)`：判断 `X-Realms-Ajax=1` 或 `Accept` 包含 `application/json`
- `writeJSON(w, status, payload)`：统一写 `Content-Type: application/json; charset=utf-8`

对当前使用 `http.Redirect(...?msg=...|?err=...)` 的 handler 增加分支：
- **AJAX:** 返回 JSON（成功/失败）；不重定向
- **非 AJAX:** 保持现有 PRG 路径（重定向 + query 提示），确保无 JS 仍可用

错误码建议：
- 参数校验失败：`400` + `{ok:false, error:"..."}`  
- 权限/未登录：沿用现有 `401/403`（前端统一提示）
- 业务失败：`400` 或 `500`（按现有语义；前端只关心 `ok`/`error`）

#### 3) 渐进启用策略（对齐 new-api 的“前后端分离”体验）
先覆盖“提交频率高且无复杂 DOM 更新需求”的页面：
- settings/users/models/announcements/subscriptions/channel-groups/channel-models
策略：
- edit 页保存：成功仅提示（不 reload）
- list 页创建/删除：成功提示 + reload（最小成本刷新列表）

#### 4) 与现有 `?msg=` 清理逻辑的关系
当前 base 模板已有 `history.replaceState` 清理 `msg` 参数，作为兼容兜底。  
在 AJAX 覆盖率足够高之前保留；当所有页面不再依赖 `?msg=` 时再评估移除（避免回归）。

## 安全与性能
- **CSRF:** 前端发送 `X-CSRF-Token`，命中中间件的 header 快路径，避免中间件提前解析 body
- **Same-Origin:** `fetch` 使用 `credentials: 'same-origin'`，不引入跨域提交
- **输出:** JSON 中的 `notice/error` 仅作为文本渲染（前端用 `textContent`），避免 XSS

## 测试与部署
- **测试:** `go test ./...`
- **回归点:** 管理后台各页面“保存/创建/删除/测试”按钮在 JS 开启时不跳转；JS 关闭时仍可通过 PRG 正常完成

