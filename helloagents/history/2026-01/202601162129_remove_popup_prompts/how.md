# 技术设计: 移除弹窗提示

## 技术方案

### 核心技术
- Go SSR 模板（`html/template`）
- 浏览器端原生 JS
- Bootstrap（现有依赖，保持不变）

### 实现要点
- 管理后台：
  - 在 `internal/admin/templates/base.html` 中注入一段尽早执行的脚本，重写 `window.alert/window.confirm/window.prompt`：
    - `alert`: 空函数（不显示、不阻塞）
    - `confirm`: 直接返回 `true`（不显示、不阻塞，等价于“自动确认”）
    - `prompt`: 返回 `null`（不显示、不阻塞）
  - 不需要逐个页面删除 inline `confirm(...)`/`alert(...)` 调用；统一在 base 层截断即可。
- 用户控制台：
  - 在 `internal/web/templates/dashboard.html` 删除“未读公告弹窗”的 DOM 结构与 `bootstrap.Modal(...).show()` 触发脚本。
  - 同步更新页面文案与管理后台公告页说明，避免“仍会自动弹出”的误导。

## 安全与性能
- **安全:** 不改动任何鉴权/权限/CSRF 逻辑；仅移除前端弹窗交互层面的确认与提示。
- **性能:** 仅增加极小的脚本常量开销；页面渲染与接口调用无变化。

## 测试与部署
- **测试:**
  - `go test ./...`
  - 手动回归：访问 `/admin/*`，触发删除/重置/排序失败等操作，确认不再出现弹窗；访问 `/dashboard`，确认不再自动弹出公告弹窗。
- **部署:** 无额外步骤（模板变更随构建发布）。

