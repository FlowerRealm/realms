# 任务清单：统一字体与字号（全局 Typography 收敛）

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 全局样式统一

- [√] 移除 SPA 中与基线不一致的 admin 专用 `code` 字体/字号覆盖（统一使用全局 `code/pre` 规则）
- [√] 新增/统一侧栏分组标题样式类（替换 inline `fontSize/letterSpacing`）

## B. 页面内手写字号/字体收敛

- [√] 替换常见页面中的 `fontSize/fontFamily` inline style 为统一的 class（优先使用现有 `small/smaller/font-monospace` 等）

## C. 验证

- [√] `cd web && npm run lint`
- [√] `cd web && npm run build`

## D. 知识库同步

- [√] `helloagents/modules/web_spa.md`：补充 Typography 统一约定（禁止随意 inline 字号/字体）
- [√] `helloagents/CHANGELOG.md`：记录本次修复
