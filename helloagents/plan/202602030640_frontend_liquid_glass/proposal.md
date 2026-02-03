# 前端重设计：Liquid Glass（亮色清新 · 动画增强）

## 目标

在**不改变任何路由语义/功能入口/后端接口对接方式**的前提下，对 Realms 前端进行一次“苹果液体玻璃（Liquid Glass）”方向的视觉与交互重设计：

- 亮色调、清新明快（浅底 + 柔和渐变光晕）
- 玻璃质感：半透明、模糊、柔和高光、细腻阴影与层级
- 动画丰富但克制：强调微交互、页面过渡与状态反馈（并支持 `prefers-reduced-motion`）
- 用户侧与管理侧保持同一视觉语言（密度不同，但质感一致）

## 硬约束（SSOT）

- 路由保持不变：参考 `FRONTEND_REDESIGN_SPEC_TMP.md` 的“路由总览”与现有 `web/src/App.tsx`。
- 功能保持不变：仅可改 UI/交互/信息呈现方式；不得删除/改写业务能力。
- 功能开关/可见性：以 `GET /api/user/self -> data.features` 为准（导航、入口、直达 URL 提示）。

## 视觉语言（设计要点）

### 背景（Light + Airy）

- 底色：`#F8FAFF`（近白偏冷）
- 光晕：多层 radial 渐变（青/靛/粉/浅绿），形成“软质彩色玻璃底”。
- 可选细节：轻噪点/微纹理（实现阶段可选，避免影响性能）。

### 玻璃表面（Glass Surfaces）

- 表面：`rgba(255,255,255,0.55~0.85)` + `backdrop-filter: blur(18~24px) saturate(1.15~1.35)`
- 边界：白色高光边（`rgba(255,255,255,0.5~0.75)`）+ 极浅暗边（`rgba(15,23,42,0.06)`）
- 阴影：软大阴影（lift）+ 小阴影（hover），避免“脏灰”
- 圆角：更“苹果化”的大圆角（18/22/26），按钮/胶囊更圆（999）

### 色彩与对比

- 主色：Violet/Indigo 系（与现有 `--bs-primary` 协调）
- 辅助：Cyan/Green/Pink 仅用于点亮状态与数据强调
- 文本：`#0F172A`（主）/ `#334155`（次）/ `#64748B`（弱）
- 所有状态要保证可读对比（尤其是玻璃背景上的文字）

## 动画与交互（实现阶段）

### 动画原则

- 以“微交互”为主：hover/press、dropdown/tooltip、toast、表单校验、加载 skeleton
- 以“液体/柔性”为基调：使用更接近 spring 的缓动曲线（`cubic-bezier(0.2, 0.8, 0.2, 1)`）
- 对 `prefers-reduced-motion: reduce` 自动降级（禁用漂浮/视差，保留必要淡入）

### 建议实现清单

- 页面切换：淡入 + 轻微上移（300~420ms）
- 玻璃卡片：hover 轻 lift + 高光增强
- 按钮：hover 微抬升 + 高光扫过（sheen），active 微回弹
- 表格行：hover 轻底色与边线增强，展开详情用 height/opacity 过渡
- loading：轻量 shimmer skeleton（禁用时降级为静态灰块）

## 交付物

- Pencil 设计稿（本方案包内）：`realms-liquid-glass.pen`
  - 已先行覆盖：`/login`、`/dashboard`、`/tokens` 的桌面版视觉方向（第一轮）
- 前端实现（`web/`）：第一轮以全局样式（背景/玻璃表面/动效）驱动整体焕新，再逐页精修

