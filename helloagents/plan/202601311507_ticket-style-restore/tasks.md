# 任务清单：工单页面样式缺失修复（对齐 SSR）

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 管理端样式对齐

- [√] `/admin/tickets`：对齐 SSR 页头按钮组容器、空状态 icon、列表卡片阴影与表格列布局（含「用户」列 icon + code）
- [√] `/admin/tickets/:id`：对齐 SSR 的返回/关闭/恢复按钮样式；消息气泡与附件 badge 的 `shadow-sm` 与 icon；回复区 card/textarea/button 样式
- [√] 修复工单列表页 tab 激活态：避免 `btn-white + text-white` 导致「全部」等字样白色不可见

## B. 验证

- [√] `cd web && npm run lint`
- [√] `cd web && npm run build`
