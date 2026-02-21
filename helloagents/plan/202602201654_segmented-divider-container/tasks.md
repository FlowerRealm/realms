# 任务清单：分段容器（外框 + 分隔线）

- [√] 定义分段容器 tokens（`--rlm-frame-*` / `--rlm-segment-sep-inset`）
- [√] 实现 `.rlm-segmented` 外框（角处留白线段）
- [√] 实现 `.rlm-segment` 内部分隔线（仅在相邻块之间）
- [√] 新增 React 组件 `SegmentedFrame`
- [√] 将 `/admin/settings` 页面改为使用分段容器（替换原 card 包裹）
- [√] 恢复 `card/table/alert/modal/dropdown` 为常规边框，避免“每个元素外面都画线”
- [√] 更新 Playwright 视觉基线并通过回归（`test:visual:update` + `test:visual`）

- [√] 扩展到所有页面的主内容区域（用户侧 + 管理侧 + 404/教程等）
- [√] 扩展视觉快照覆盖所有路由页面（新增 `/guide`、`/tokens/created`、`/admin/payment-channels`、`/admin/channel-groups/:id`、NotFound）
- [√] 同步模块文档（`helloagents/modules/web-theme.md`）
