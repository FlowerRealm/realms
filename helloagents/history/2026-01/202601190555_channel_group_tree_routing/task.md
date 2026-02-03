# 任务清单: 渠道组树形路由（Channel Group Routing）

目录: `helloagents/plan/202601190555_channel_group_tree_routing/`

---

## 1. 数据模型与迁移
- [√] 1.1 新增迁移 `internal/store/migrations/0033_channel_group_members.sql`：创建 `channel_group_members` 并为 `channel_groups` 增加 `max_attempts`，验证 why.md#需求-树形渠道组default-根组-场景-root-在-default-下创建子渠道组并进入
- [√] 1.2 编写回填逻辑：从 `upstream_channels.groups` 迁移生成成员关系（`default -> 所有现有组` + `组 -> 渠道`），并对缺失的组名做兼容处理，验证 why.md#需求-数据面递归调度与-failover-场景-请求从-default-递归选择可用叶子渠道遵守约束

## 2. store 层（SSOT + 兼容缓存）
- [√] 2.1 在 `internal/store/channel_groups.go` 增加 `channel_group_members` 的读写接口（列出成员、增删成员、重排 priority、读取/更新 group.max_attempts），验证 why.md#需求-树形渠道组default-根组-场景-渠道可被多个组引用仅叶子复用
- [√] 2.2 维护 `upstream_channels.groups` 兼容缓存：成员关系变更后回填更新，保证现有模板/查询不被破坏，验证 why.md#需求-树形渠道组default-根组-场景-渠道可被多个组引用仅叶子复用

## 3. scheduler：渠道级自动 ban
- [√] 3.1 扩展 `internal/scheduler/state.go`：增加 channel ban 状态（streak + until）与查询/更新方法，验证 why.md#需求-自动-ban-渠道-场景-渠道连续可重试失败后进入-ban并被后续选择跳过
- [√] 3.2 调整 `internal/scheduler/scheduler.go` 的 `Report`：对 retriable 失败叠加 ban 时长、成功清零，且在选择时跳过 ban 渠道，验证 why.md#需求-自动-ban-渠道-场景-渠道连续可重试失败后进入-ban并被后续选择跳过

## 4. 数据面递归调度（Group Router）
- [√] 4.1 新增递归路由组件（建议 `internal/scheduler/group_router.go` 或独立包）：从 `default` 根组按 `max_attempts` 递归选叶子渠道（去重/visited 防护），验证 why.md#需求-数据面递归调度与-failover-场景-子组耗尽后回退到同级下一个成员
- [√] 4.2 改造 `internal/api/openai/handler.go`：替换当前“全局 maxAttempts=5”循环为“根组递归调度”；保持 SSE 写回后禁止 failover 的边界不变，验证 why.md#需求-数据面递归调度与-failover-场景-sse-写回后禁止-failover

## 5. 管理后台（树形 UI）
- [√] 5.1 改造 `/admin/channels`（或新增 group 详情页）模板与 handler：展示当前组成员、可递归进入子组、支持创建子组/创建渠道/引用渠道、并支持成员排序与组 `max_attempts` 配置，验证 why.md#需求-树形渠道组default-根组-场景-root-在-default-下创建子渠道组并进入
- [√] 5.2 增加写入校验：子组单父与防环检测（仅叶子可复用），验证 why.md#需求-树形渠道组default-根组-场景-渠道可被多个组引用仅叶子复用

## 6. 安全检查
- [√] 6.1 执行安全检查（输入校验、权限控制、避免环/重复写导致的 DoS；ban 叠加做溢出保护），并补齐关键错误信息，验证 why.md#风险评估

## 7. 文档更新（知识库同步）
- [√] 7.1 更新 `helloagents/wiki/data.md`：补齐 `channel_group_members` 与 `channel_groups.max_attempts`，验证 why.md#变更内容
- [√] 7.2 更新 `helloagents/wiki/modules/realms.md`：记录路由树与 ban 行为、并补齐变更历史条目，验证 why.md#变更内容

## 8. 测试
- [√] 8.1 新增/更新单测覆盖递归调度与 ban 行为（优先 `internal/scheduler/*_test.go` 与 `internal/api/openai/*_test.go`），并执行 `go test ./...`，验证 why.md#核心场景
