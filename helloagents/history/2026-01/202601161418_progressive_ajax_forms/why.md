# 变更提案: 渐进式 AJAX 表单提交（不跳转 / 不污染 URL）

## 需求背景
当前管理后台与部分用户控制台页面使用“POST 后重定向（PRG）+ `?msg=` / `?err=`”来展示一次性提示消息。  
这会导致地址栏出现后缀参数，并在频繁保存/创建/删除时污染浏览器历史记录，影响“上一页/下一页”体验。

目标是借鉴 new-api 的思路：**页面通过 AJAX 提交到服务端，服务端返回 JSON，前端在原页面展示提示（toast/alert）**，从而做到：
1) 提交后不发生页面跳转；2) 提示信息不进入 URL；3) 仍保留无 JS 的兜底路径（PRG 不破坏）。

## 变更内容
1. 增加前端通用脚本：拦截标记为 `data-ajax="1"` 的表单提交，使用 `fetch` 提交并在页面展示成功/失败提示。
2. 服务端为相关 POST handler 增加 AJAX 分支：识别 `X-Realms-Ajax: 1`（或 `Accept: application/json`），成功返回 `{ok:true, notice:"..."}`，失败返回 `{ok:false, error:"..."}`，不再重定向。
3. 逐页渐进开启：先覆盖管理后台高频表单（settings/users/models/announcements/subscriptions/channel-groups/channel-models 等），再按需要扩展到用户控制台。

## 影响范围
- **模块:** `internal/admin`、`internal/web`（仅 Web UI 与管理后台；数据面 `/v1/*` 不受影响）
- **文件:** SSR 模板（base + 各页面表单）、相关 POST handler
- **API:** 新增仅内部使用的“返回 JSON”分支（同一路径，按 header 分流；不新增对外公开 API）
- **数据:** 无数据库变更

## 核心场景

### 需求: 管理后台表单提交不跳转
**模块:** admin

#### 场景: 保存/创建/删除后停留在原页面
- 点击“保存/创建/删除”后页面不跳转，地址栏不出现 `?msg=`/`?err=`。
- 页面展示成功/失败提示（toast 或 alert）。
- 必要时页面可轻量刷新（`location.reload()`），但不新增历史记录条目。

### 需求: 兼容无 JS / 失败兜底
**模块:** admin/web

#### 场景: JS 关闭或 fetch 失败
- 无 JS 时仍走现有 PRG（重定向）流程，功能不回退。
- fetch 失败时提示“网络错误/请重试”，不破坏页面状态。

## 风险评估
- **风险:** 表单重复提交（用户连点/网络抖动）  
  **缓解:** 前端提交期间禁用按钮；服务端仍保持幂等/校验（至少不崩溃）。
- **风险:** CSRF / 跨域误用  
  **缓解:** 使用现有 CSRF 中间件；前端携带 `X-CSRF-Token`；`credentials: 'same-origin'`。
- **风险:** 渐进迁移期行为不一致  
  **缓解:** 仅对显式标记 `data-ajax="1"` 的表单启用；未标记保持原行为。

