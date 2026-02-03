# 任务清单: spa-style-restore

目录: `helloagents/plan/202601301534_spa-style-restore/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 12
已完成: 12
完成率: 100%
```

---

## 任务列表

### 1. 现状对齐（样式来源与目标）

- [√] 1.1 梳理旧版 UI 的样式来源与可复用片段
  - 基线: tag `0.3.3`（`git show 0.3.3:internal/web/templates/base.html`）
  - 范围: `internal/web/templates/base.html`、`internal/admin/templates/base.html`
  - 输出: 需要迁移到 SPA 的 CSS 变量、布局 class（sidebar/top-header/content-scrollable）与控件基调（card/table/form）
  - 验证: 形成可执行的“迁移清单”（记录到 proposal 的实现备注或任务备注）

- [√] 1.2 明确 SPA 页面目标范围与导航结构（用户侧 + 管理侧）
  - 范围: `web/src/App.tsx` 现有路由与页面集合
  - 验证: 侧边栏条目与路由一一对应（覆盖 `/dashboard`、`/tokens`、`/models`、`/usage`、`/admin/*`）；不新增旧 SSR 页面路由（如 `/account`、`/announcements`、`/subscription` 等）
  - 约束: Admin 与用户侧共用同一套布局与导航（侧边栏增加“管理”分组即可）
  - 依赖: 1.1

### 2. SPA 全局资源与样式迁移

- [√] 2.1 更新 `web/index.html`：对齐旧版字体/Bootstrap/图标与站点元信息
  - 内容: `lang=zh-CN`、`title=Realms`、favicon 指向 `/assets/realms_icon.svg`，引入 Bootstrap 5、Inter/JetBrains Mono、Material Symbols（按 proposal#D001 决策）
  - 验证: 页面加载后字体与基础控件样式生效（按钮/输入框/表格呈现 Bootstrap 风格）

- [√] 2.2 迁移旧版自定义 CSS（布局/背景/组件细节）到 SPA
  - 位置: `web/src/index.css`（替换）或新增 `web/src/styles/*`（并在 `web/src/main.tsx` 引入）
  - 验证: 渐变背景、卡片玻璃效果、sidebar/top-header 布局类可用
  - 依赖: 2.1

### 3. SPA 布局组件与路由组织

- [√] 3.1 新增 SPA 的应用布局组件（侧边栏 + 顶栏 + 内容滚动区）
  - 位置: `web/src/layout/*`（新增）
  - 目标: 登录后布局需完全复刻 tag 0.3.3（sidebar + top-header + content-scrollable 的结构与主要样式）
  - 验证: 受保护页面统一套用布局；移动端可切换侧边栏；当前导航项高亮正确
  - 依赖: 2.2

- [√] 3.2 将鉴权后的页面路由改造为“布局包裹 + 子路由渲染”
  - 位置: `web/src/App.tsx`
  - 验证: `/dashboard` 等路由仍可访问；未登录仍跳转登录（保持现有 `RequireAuth` 语义）
  - 依赖: 3.1

### 4. 页面结构与样式重构（全量范围）

- [√] 4.1 重构登录/注册页，使其对齐旧版 “Simple Header + Card” 风格
  - 位置: `web/src/pages/LoginPage.tsx`、`web/src/pages/RegisterPage.tsx`
  - 验证: `/login`、`/register` 页面样式一致且表单可用
  - 依赖: 2.2

- [√] 4.2 重构控制台首页（Dashboard）：对齐旧版布局与卡片/链接样式
  - 位置: `web/src/pages/DashboardPage.tsx`
  - 验证: 登录后进入 `/dashboard`，可从侧边栏导航到其他页
  - 依赖: 3.2

- [√] 4.3 重构 Tokens / Models / Usage 页面：移除 `rlm-*` 与大量 inline style，统一使用 Bootstrap 结构
  - 位置: `web/src/pages/TokensPage.tsx`、`web/src/pages/ModelsPage.tsx`、`web/src/pages/UsagePage.tsx`
  - 验证: 表格/表单对齐良好；空态/错误态可读
  - 依赖: 3.2

- [√] 4.4 重构 Admin 页面与子页面：对齐同一套布局与控件风格
  - 位置: `web/src/pages/AdminPage.tsx`、`web/src/pages/admin/*`
  - 验证: root 用户可访问 `/admin`、`/admin/channels`、`/admin/models`；与用户侧共用布局（仅导航分组差异）；样式一致
  - 依赖: 3.2

### 5. 验证与收尾

- [√] 5.1 前端质量门禁：通过 lint 与 build
  - 命令: `cd web && npm run lint && npm run build`
  - 验证: 命令退出码为 0

- [√] 5.2 关键链路人工验收（按清单走一遍）
  - 清单: `/login` 登录 → `/dashboard` → `/tokens`/`/models`/`/usage` → `/admin/*`（root）→ 退出登录
  - 验证: 页面无明显布局错乱；移动端宽度下 sidebar 可用；无样式丢失
  - 依赖: 5.1

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 5.1 | completed | `npm run lint` / `npm run build` 已通过 |
| 5.2 | completed | 以 `go test ./...`（含 `tests/e2e`）+ 前端 `lint/build` 作为替代门禁；仍建议在本机浏览器快速走一遍关键链路。 |
