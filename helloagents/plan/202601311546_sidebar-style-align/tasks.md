# 任务清单：用户侧左侧栏样式对齐管理侧

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 侧栏一致性修复

- [√] `AppLayout`：移除 `ul` 的额外 `gap`，结构与管理侧对齐
- [√] `index.css`：用户侧与管理侧共用同一套 `.sidebar-link` spacing（padding/margin/radius）
- [√] 回归检查：hover/active 状态文字颜色一致（避免被 admin 链接样式覆盖）

## B. 验证

- [√] `cd web && npm run lint`
- [√] `cd web && npm run build`

## C. 知识库同步

- [√] `helloagents/modules/web_spa.md`：补充“侧栏样式统一”的约定
- [√] `helloagents/CHANGELOG.md`：记录本次修复
