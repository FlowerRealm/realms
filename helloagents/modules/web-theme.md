# web-theme

## 概述

本模块记录前端（`web/`）的主题配色约定与回归检查点，目标是保证“主强调色”为浅绿色系，并避免落回 Bootstrap 默认亮蓝色。

---

## 主题主色（Primary）

主题 SSOT：`web/src/index.css`

- 主色：`--bs-primary` / `--bs-primary-rgb`
- Hover/Active 深色档：`--rlm-primary-600` / `--rlm-primary-700` / `--rlm-primary-800`
- 相关派生：`--bs-link-color`、`--bs-primary-bg-subtle`、`--bs-primary-border-subtle`

---

## 角色化 Tokens（推荐）

主题 SSOT：`web/src/index.css` 的 `Theme Tokens (SSOT)` 段落。

用于“清晰统一”背景/文字/主题/交互/警示的命名（保持鼠尾草绿系不变）：

- 背景：`--rlm-bg`（以及 `--rlm-bg-rgb`）
- 文字：`--rlm-text` / `--rlm-text-muted`（以及 `--rlm-text-rgb` / `--rlm-text-muted-rgb`）
- 标题：`--rlm-heading`（以及 `--rlm-heading-rgb`）
- 容器面：`--rlm-surface-1` / `--rlm-surface-2` / `--rlm-surface-3`（以及 `--rlm-surface-3-hover`）
- 边界：`--rlm-border` / `--rlm-border-soft`
- 主题色：`--rlm-primary` / `--rlm-primary-rgb`（并保留 `--rlm-primary-600/700/800` 阶梯）
- 交互（Focus）：`--rlm-focus-ring` / `--rlm-focus-ring-strong`

当前选定版本：**B（更白、更疏离、更可信赖）**。

---

## Bootstrap 覆盖点

### `nav-pills` 激活态

Bootstrap 的 `nav-pills` 激活态在部分情况下可能呈现默认亮蓝色，因此在主题层做全局覆盖，确保如 `/admin/settings` 的 tabs 激活态始终命中主题主色：

- `.nav-pills { --bs-nav-pills-link-active-bg: var(--bs-primary); --bs-nav-pills-link-active-color: #fff; }`
- `.nav-pills .nav-link.active` / `.nav-pills .show > .nav-link` 明确设置 `background-color` / `color`
- 统一描边：`.nav-pills .nav-link` 默认带淡边框；active 时边框随主色加深（避免“有的 tabs 有框、有的没框”）

### “胶囊/标签”淡边框一致性

为解决“有些文字外面有淡淡的框、有些没有”的不一致，全站做如下约定（SSOT：`web/src/index.css`）：

- `.rounded-pill` 默认带淡边框（覆盖非 `.badge` 的 pill 容器/标签）
- `.badge` 保持统一淡边框（避免不同页面写法差异）
- 如确实需要无边框，显式加 `border-0`（避免误把“缺失边框”当作设计不一致）

### 交互控件淡边框一致性

为减少“有的按钮/区域有框、有的没框”的割裂感（SSOT：`web/src/index.css`）：

- `.rlm-segmented` 默认带淡边框（作为页面主要区域外框）
- `.btn` 默认带淡边框（与输入框一致）；`.btn-link` 例外保持无边框
- `.nav-tabs` / `.dropdown-item` / `.list-group` 统一淡边框（避免局部无框）
- `.rounded-circle` 与常见 `bg-* + rounded-*` 的信息块默认带淡边框（避免图标/小面板“无框漂浮”）

---

## 回归检查

- 静态扫描（禁止亮蓝色字面量）：`npm -C web run check:theme`
  - 脚本：`web/scripts/check-theme-colors.js`
- E2E 回归（主题色不为亮蓝，且命中当前主色 RGB）：`npm -C web run test:e2e:ci -- e2e/theme-colors.spec.ts`
  - 用例：`web/e2e/theme-colors.spec.ts`
- 视觉快照回归（全站页面截图）：`npm -C web run test:visual`
  - 更新基线：`npm -C web run test:visual:update`
  - 用例：`web/e2e/visual-routes.spec.ts`（为稳定性会屏蔽远程 fonts/icon CSS，仅验证布局/层级/配色）
- 关键断言：`/dashboard` 与 `/admin` 会额外校验侧栏链接颜色层级（未激活=muted，激活=heading），并单独生成侧栏快照 `app-sidebar.png` / `admin-sidebar.png`

---

## 布局面与分隔（登录后）

为保持“轻盈 + 距离感”，登录后主框架采用白色内容面，并用淡实线分隔结构区：

- 内容区背景：`.content-scrollable { background: var(--rlm-surface-1); }`
- 顶栏分隔：`.top-header { border-bottom: 1px solid var(--rlm-border-soft); }`
- 侧栏分隔：`.sidebar { border-right: 1px solid var(--rlm-border-soft); }`
- 侧栏导航文字：默认 `--rlm-muted`；hover/active 使用 `--rlm-heading`（不再使用主色绿，改用背景高亮表达当前项）
- 侧栏导航框线：侧栏例外，`.sidebar-link` 不绘制淡边框（避免每个条目都像“单独一个框”）；仅用背景高亮表达 hover/active
- 圆角统一：`--bs-border-radius` 映射到 `--rlm-frame-radius`，避免输入框/按钮/聚焦框出现方角割裂

---

## 分段容器（外框 + 分隔线）

为满足“不是每个元素外面都需要线，而是块与块分隔处需要线；同时整体外侧仍需要线包住”的诉求，新增分段容器样式：

- SSOT：`web/src/index.css` 的 `.rlm-segmented` / `.rlm-segment` 与 `--rlm-frame-*` tokens
- React 封装：`web/src/components/SegmentedFrame.tsx`

约定：

- 外框：不绘制（最外层不需要任何实线）
- 内部分隔：仅在相邻块之间绘制水平分隔线（两端留白），由 `.rlm-segment + .rlm-segment::before` 绘制
- 分隔线长度：由 `--rlm-segment-sep-inset` 控制（值越小，线越长）
- 同段自动分隔：同一 `.rlm-segment` 内，若使用 `.row` 纵向堆叠多个 `.col-12`，会自动在每个非首个 `.col-12` 之前补分隔线并增大留白（用于减少“同页有的地方有线、有的地方没有”的不一致）
- 使用方式：页面主内容优先用 `<SegmentedFrame>` 包裹多个“区域块”（children），由容器统一负责分隔与外框

---

## 段内强制分隔（DividedStack）

当单个 segment 内部结构复杂（例如多层 `row/col`、条件渲染较多）导致“同段自动分隔”无法稳定命中时，使用 `DividedStack` 显式把纵向区块拆成多个 item，由样式统一绘制分隔线与留白：

- React 封装：`web/src/components/DividedStack.tsx`
- 样式：`web/src/index.css` 的 `.rlm-divided-stack` / `.rlm-divided-item`
- 约束：容器会忽略纯空白文本子节点（避免因 JSX 换行/缩进产生空分隔条）

---

## 相关文件

- `web/src/index.css`
- `web/src/components/SegmentedFrame.tsx`
- `web/src/components/DividedStack.tsx`
- `web/scripts/check-theme-colors.js`
- `web/e2e/theme-colors.spec.ts`
- `web/e2e/visual-routes.spec.ts`
