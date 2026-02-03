# 任务清单：Liquid Glass 前端重设计（第一轮）

> 说明：本任务以“全局视觉系统 + 关键页面示范”为第一轮目标；后续按页面逐步覆盖，但始终保持路由与功能不变。

## 设计（Pencil）

- [√] 设计方向建立：亮色清新 + 多层光晕背景 + 玻璃表面（卡片/窗口/胶囊）
- [√] 关键页面示范（Desktop）
  - [√] `/login`（Public Layout + 登录卡片）
  - [√] `/dashboard`（App Layout + 指标卡片 + 图表卡片 + 公告卡片）
  - [√] `/tokens`（App Layout + 令牌表格卡片）
- [ ] 补齐页面示范
  - [ ] `/register`
  - [ ] `/usage`
  - [ ] `/account`
  - [ ] `/admin`（概览 + 数据密度方案）
  - [ ] `/oauth/authorize`

## 前端实现（web/）

- [√] 建立 Liquid Glass 设计 token（CSS variables）
- [√] 背景升级：多层 radial 渐变光晕（替代单一线性渐变）
- [ ] 玻璃表面统一：`card` / `sidebar` / `top-header` / `dropdown` / `modal`（已覆盖前四项，modal 待补齐）
- [ ] 动效升级：按钮、卡片、列表、dropdown、页面过渡（含 `prefers-reduced-motion`）（已覆盖基础动效，逐页细化中）
- [ ] 首批落地页面（保证功能不变）
  - [ ] `/login`、`/register`
  - [ ] `/dashboard`
  - [ ] `/tokens`

## 验证

- [ ] `web/` 构建通过：`npm -C web run build`
- [ ] E2E 冒烟（可选）：`npm -C web run test:e2e`
