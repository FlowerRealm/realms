# 方案提案：全项目统一“老的时区选择样式”（datalist 提示）

## 背景

项目从 SSR 迁移到 SPA 后，管理后台的“系统时区”输入框在 SPA 侧变成了普通文本输入，缺少旧版的时区候选提示（datalist），导致输入体验退化。

本轮以 SSR 模板为真实性基准（SSOT），将“老的时区选择样式”复刻到 SPA，并以可复用组件方式对全项目生效。

## 页面差异计划

页面：`/admin/settings`（管理后台：系统设置 - 基础设置 - 系统时区）
  原来的内容：`internal/admin/templates/settings.html`
    - 使用 `<input type="text" list="adminTimeZones" ... />` + `<datalist id="adminTimeZones">...`
    - 具备常用时区候选提示（如 `Asia/Shanghai`、`UTC` 等）
  现在的内容：`web/src/pages/admin/SettingsAdminPage.tsx`
    - 仅为普通 `<input class="form-control" />`，缺少 datalist 候选提示
  差异：
    - 需要把旧版的 datalist 候选提示样式复刻到 SPA
    - 为“对整个项目应用”，应抽成可复用组件，未来所有时区输入统一复用（避免重复实现/样式漂移）

