# 任务清单：全站移除页面级“返回/返回列表”按钮

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 全站移除“返回按钮”

- [√] 用户侧：移除 `/pay`、`/tickets/:id`、`/announcements/:id`、`/tokens/created`、`/models` 的页面级返回按钮
- [√] 管理侧：移除 `/admin/*` 各列表页与详情页的页面级返回/返回列表按钮（保留 breadcrumb/侧栏导航）
- [√] 404：将“返回控制台/返回登录”改为“前往控制台/前往登录”

## B. 上游渠道：统计区间自动刷新

- [√] `/admin/channels`：移除“查询/更新统计”按钮，改为修改日期后自动更新（统计区间仍保持 row g-2、始终展开）

## C. 验证

- [√] `cd web && npm run lint`
- [√] `cd web && npm run build`
- [√] `go test ./...`

## D. 知识库同步

- [√] `helloagents/modules/web_spa.md`：补充“页面不提供返回按钮”的 UI 约定
- [√] `helloagents/CHANGELOG.md`：记录本次变更
