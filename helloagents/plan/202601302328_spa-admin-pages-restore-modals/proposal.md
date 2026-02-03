# 方案提案：SPA 管理后台页面修复（弹窗交互 + 中文化 + 缺失功能补齐）

## 背景

在从 SSR 管理后台迁移到 SPA（`web/`）之后，出现了以下问题：

- 多个管理页面的“新增/编辑”交互从原先的 Bootstrap modal 退化成页面内表单/独立卡片，导致体验与基线不一致。
- 部分页面文案仍为英文或中英混杂（例如 OAuth Apps）。
- 个别 SSR 功能在 SPA 侧缺口明显（例如模型管理的“从模型库填充/导入价格表”、渠道页的“渠道指针”信息）。

本方案以 `internal/admin/templates/*.html` 为“原来的内容”基准，优先恢复 SPA 管理后台的关键页面交互与功能闭环。

## 目标（本轮）

1. **页面中文化**：管理页面的标题/按钮/提示语以中文为主（保留必要技术术语，如 `client_id`、`redirect_uri`）。
2. **弹窗交互一致**：新增/编辑/导入等操作尽量使用 Bootstrap modal（小窗），对齐 SSR 交互。
3. **补齐关键缺失功能**：
   - 模型管理：从 `models.dev` 填充定价/归属方；导入价格表。
   - 渠道管理：展示“智能调度：渠道指针”，并支持“设为指针”；补齐按渠道统计、封禁/失败计分运行态、拖拽排序。
4. **前后端接口对齐**：为上述 UI 补齐缺失的 JSON API，并保持 `APIResponse<{...}>` 风格一致。

## 范围

### 覆盖页面（SPA）

- `/admin/payment-channels`（支付渠道）
- `/admin/subscriptions`（订阅套餐）
- `/admin/oauth-apps`、`/admin/oauth-apps/:id`（OAuth 应用）
- `/admin/models`（模型管理）
- `/admin/channels`（上游渠道管理：渠道指针）

### 对照基线（SSR）

- `internal/admin/templates/payment_channels.html`
- `internal/admin/templates/subscriptions.html`
- `internal/admin/templates/oauth_apps.html`、`oauth_app.html`、`oauth_app_secret.html`
- `internal/admin/templates/models.html`
- `internal/admin/templates/channels.html`

## 页面差异与修复计划（SSR 基线 vs SPA 现状）

### 页面：`/admin/payment-channels`

- 原来的内容：`internal/admin/templates/payment_channels.html` 中以表格展示渠道列表；新增/编辑均为 Bootstrap modal（弹出小窗）；页面文案为中文。
- 现在的内容：SPA 迁移后部分交互退化为页面内表单/卡片；部分样式/按钮结构与 SSR 不一致。
- 差异：
  - “新增/编辑/保存”交互需要回归 modal。
  - 文案统一中文（保留必要技术字段）。

### 页面：`/admin/subscriptions`

- 原来的内容：`internal/admin/templates/subscriptions.html` 支持在列表页通过 modal 新增套餐；编辑/保存体验一致；文案中文。
- 现在的内容：SPA 迁移后新增表单出现在页面内；交互与 SSR 不一致。
- 差异：
  - “新增套餐”应使用 modal（小窗）完成。
  - 列表与按钮样式需对齐 SSR 的 Bootstrap 结构。

### 页面：`/admin/oauth-apps`

- 原来的内容：`internal/admin/templates/oauth_apps.html` 支持 modal 新建应用；空状态/按钮为中文；字段说明清晰。
- 现在的内容：SPA 中存在英文或中英混杂；新增交互与 SSR 不一致。
- 差异：
  - 页面标题、空状态、按钮文案统一中文。
  - “新建应用”回归 modal。

### 页面：`/admin/oauth-apps/:id`

- 原来的内容：`internal/admin/templates/oauth_app.html` + `oauth_app_secret.html` 提供应用详情与 Secret 管理；说明为中文；保留技术字段名。
- 现在的内容：SPA 中部分文案英文；Secret 管理不够对齐（缺少“生成/轮换”等明确动作）。
- 差异：
  - 文案中文化（技术字段保留）。
  - Secret 管理按钮与提示对齐 SSR（突出“生成/轮换 Secret”）。

### 页面：`/admin/models`

- 原来的内容：`internal/admin/templates/models.html` 提供模型白名单管理；支持“导入价格表（JSON）”；并提供“从模型库填充”（用于快速填充 `owned_by` 与定价）。
- 现在的内容：SPA 中缺少“从模型库填充/导入价格表”或体验不完整；导入后难以看清哪些条目新增/更新/失败。
- 差异：
  - 补齐 models.dev 查询填充入口（含图标预览）。
  - 导入价格表使用 modal，并在 UI 中展示详细结果：新增/更新/无变化/失败（含失败原因）。

### 页面：`/admin/channels`

- 原来的内容：`internal/admin/templates/channels.html` 展示渠道列表；支持按日期区间统计（消耗/Token/缓存命中率）；展示运行态（封禁状态等）；支持拖拽排序（Sortable）；新增/设置均使用 modal；页面文案中文。
- 现在的内容：SPA 渠道页存在能力缺口：缺少按渠道统计区间、运行态封禁/失败计分展示、拖拽排序等；部分文案仍偏技术化或英文。
- 差异：
  - 增加日期区间筛选，并在列表内展示“消耗/Token/缓存命中率”。
  - 展示运行态：封禁状态（剩余时间/封禁至）与 fail score。
  - 支持拖拽排序并持久化到后端（与 SSR 一致的“越靠前优先级越高”）。

## 非目标（暂不在本轮完成）

- 完整复刻 `/admin/endpoints` 与 Codex OAuth 账号管理（SSR `endpoints.html` 内容较大，需要额外 API 设计）。
- 完整复刻 `/admin/channels/:id/detail` 的按 key/账号用量聚合展示（SSR `channel_detail.html`）。
- 为所有页面补齐“可视化柱形背景/快捷日期选择”等纯 UI 增强（可作为后续迭代）。

## 实施要点

- 引入可复用组件 `web/src/components/BootstrapModal.tsx` 统一 modal 结构，并提供 `web/src/components/modal.ts` 关闭 helper，避免重复写原生 Bootstrap JS 操作。
- 在 Router/API 侧新增：
  - `POST /api/models/library-lookup`（models.dev 查询）
  - `POST /api/models/import-pricing`（导入定价 JSON）
  - `GET /api/channel/pinned`（渠道指针查询）
  - `POST /api/channel/:channel_id/promote`（设为指针）
  - `GET /api/channel/page`（渠道页：按日期区间统计 + 运行态汇总）
  - `POST /api/channel/reorder`（渠道拖拽排序持久化）
- 对于“渠道指针”依赖运行态 scheduler 的能力，通过 `internal/admin` 提供面向 API 的桥接方法（不走 SSR CSRF 体系）。

## 验证与回归

- `go test ./...`
- `cd web && npm run build`
- `cd web && npm run lint`
