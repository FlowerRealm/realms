# 方案提案：统一字体与字号（全局 Typography 收敛）

## 背景

迁移到 SPA 后，项目中出现了多处“局部手写字号/字体”的实现（例如 `style={{ fontSize: ... }}`、`style={{ fontFamily: ... }}`），导致不同页面/模块间的字体与字号不一致，观感割裂。

本轮以旧版 SSR 的全局基线（`internal/web/templates/base.html`、`internal/admin/templates/base.html`）为参考，统一 SPA 的字体族与字号策略，减少“局部特殊处理”。

## 页面差异计划

页面：全站（用户侧 + 管理侧，SPA）
  原来的内容：
    - SSR 基线：`body` 使用统一的 sans 字体（Inter）与统一的基础字号（0.9rem）
    - `code/pre` 统一使用 `--bs-font-monospace`（JetBrains Mono）与一致字号（0.875em）
  现在的内容：
    - `web/src/index.css` 中存在与全局不一致的 admin 专用 `code` 字体/字号覆盖
    - 多个页面存在手写 `fontSize/fontFamily`（例如 Dashboard/Token/Usage/Orders/Settings 等）
  差异：
    - 需要移除/收敛这些“局部字号/字体”覆盖，统一走全局 CSS 与 Bootstrap 的一致策略

