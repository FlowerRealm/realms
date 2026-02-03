# 任务清单：全项目统一“老的时区选择样式”

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 时区输入组件化

- [√] 新增 `TimeZoneInput` 组件：复刻 SSR 的 `input[list] + datalist` 交互与候选项
- [√] 替换管理后台设置页的时区输入为 `TimeZoneInput`

## B. 验证

- [√] `cd web && npm run lint`
- [√] `cd web && npm run build`

## C. 知识库同步

- [√] `helloagents/modules/web_spa.md`：补充“时区选择样式（datalist）”约定
- [√] `helloagents/CHANGELOG.md`：记录本次修复
