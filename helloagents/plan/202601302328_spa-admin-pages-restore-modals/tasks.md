# 任务清单：SPA 管理后台页面修复（弹窗交互 + 中文化 + 缺失功能补齐）

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 页面交互对齐（modal）

- [√] `/admin/payment-channels`：新建/编辑改为 modal，页面内表单移除
- [√] `/admin/subscriptions`：新增套餐改为 modal，页面内表单移除
- [√] `/admin/oauth-apps`：新建应用改为 modal；标题/空状态中文化
- [√] `/admin/oauth-apps/:id`：标题/字段说明中文化（保留技术字段名）

## B. 模型管理补齐

- [√] `/admin/models`：新增“导入价格表”入口与 modal
- [√] `/admin/models`：新增“从模型库填充（models.dev）”按钮与图标预览
- [√] 导入价格表：在 UI 中展示详细结果（新增/更新/无变化/失败列表 + 失败原因）

## C. 渠道页补齐（指针能力）

- [√] `/admin/channels`：展示“智能调度：渠道指针”卡片（含 hover note）
- [√] `/admin/channels`：增加“设为指针”按钮（调用 API 并刷新）
- [√] 补齐更多 SSR 渠道设置项与运行态数据（fail score/ban 状态/按渠道统计/拖拽排序等）

## D. 后端 API 对齐

- [√] `POST /api/models/library-lookup`
- [√] `POST /api/models/import-pricing`
- [√] `GET /api/channel/pinned`
- [√] `POST /api/channel/:channel_id/promote`
- [√] `GET /api/channel/page`
- [√] `POST /api/channel/reorder`

## E. 验证

- [√] `go test ./...`
- [√] `cd web && npm run build`
- [√] `cd web && npm run lint`
