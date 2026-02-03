# 方案提案：用户侧左侧栏样式对齐管理侧（统一 Sidebar 观感）

## 背景

技术库迁移到 SPA（`web/`）后，用户侧与管理侧都采用“sidebar + top-header + content-scrollable”的应用布局，但左侧栏（sidebar）的间距/按钮观感不一致，影响整体统一性。

本轮以“管理侧左侧栏”为基线，将用户侧左侧栏对齐到同一套样式与结构。

## 页面差异计划

页面：用户侧应用布局（`/dashboard`、`/tokens`、`/usage`、`/tickets` 等，`AppLayout`）
  原来的内容：
    - 侧栏菜单项（`.sidebar-link`）更“松”（更大的 padding/margin/radius）
    - 列表额外使用 `gap` 增加条目间距
  现在的内容：
    - `web/src/layout/AppLayout.tsx`：`ul` 使用 `gap: 0.1rem`，导致与管理侧列表密度不同
    - `web/src/index.css`：`.sidebar-link` 默认采用 web_base 的 spacing（padding 0.75 / margin 0.4 / radius 0.6）
  差异：
    - 菜单条目密度（padding/margin/radius）与管理侧不一致
    - 结构上（`ul` 的 gap）与管理侧不一致

页面：管理侧应用布局（`/admin/*`，`AdminLayout`）
  原来的内容：
    - 管理侧 `.sidebar-link` 更紧凑（padding 0.6 / margin 0.2 / radius 0.5）
    - 列表未额外使用 `gap`
  现在的内容：
    - `web/src/index.css` 存在 `.admin-html .sidebar-link` 的紧凑样式，但用户侧未对齐
  差异：
    - 用户侧未复用管理侧的紧凑 spacing，导致两套左侧栏“看起来不像同一个系统”

## 修复策略

- 统一用户侧与管理侧的 `.sidebar-link` spacing：使用同一套紧凑值（对齐管理侧）
- 移除用户侧 `ul` 的额外 `gap`，避免与管理侧密度不一致

