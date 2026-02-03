# 任务清单：上游渠道页面 UI 优化

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 页面 UI 调整（/admin/channels）

- [√] 弱化“查询”入口：把“查询统计”改为“统计区间（可选）/更新统计”，并减少干扰性文案
- [√] 页头结构对齐 SSR：去掉不必要的卡片化包装与“返回”按钮，提升信息密度与一致性
- [√] 统计区间控件对齐 SSR：放回页头下方（row g-2），始终展开；移除时区文本显示
- [√] 渠道详情区域去掉不必要的“文字外框”（code/badge），减少占用空间（保留必要信息）

## B. Modal 灰幕问题修复（全局）

- [√] `BootstrapModal` 使用 Portal 渲染到 `document.body`（对齐 SSR 的“modal 挂到 body”策略）

## C. 验证

- [√] `cd web && npm run lint`
- [√] `cd web && npm run build`

## D. 知识库同步

- [√] `helloagents/modules/web_spa.md`：补充“modal 必须挂到 body（Portal）”约定
- [√] `helloagents/CHANGELOG.md`：记录本次修复
