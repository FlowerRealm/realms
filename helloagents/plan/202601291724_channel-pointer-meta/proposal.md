# 变更提案: channel-pointer-meta

## 元信息
```yaml
类型: 优化
方案类型: implementation
优先级: P2
状态: 已完成
创建: 2026-01-29
```

---

## 1. 背景与问题

当前管理后台已将“渠道指针”作为“应该优先使用哪个渠道”的唯一标定，但页头只展示指针目标渠道本身，缺少指针最近一次变更的可解释性信息：
- 什么时候变更（更新时间）
- 为什么变更（手动设置 / 因封禁轮转 / 指针无效自动修正）

这会导致排障时只能看到“现在指向谁”，但看不到“为何变成这样”。

---

## 2. 目标

- `/admin/channels` 页头在“渠道指针”徽标上补齐指针变更信息（建议使用 `title` 提示，避免占用布局）。
- 调度器在“手动设置/清除指针”时同步记录 `moved_at/reason`，并保持现有“封禁触发自动轮转/无效指针自动修正”的原因记录。

---

## 3. 方案

### 3.1 Scheduler（`internal/scheduler/`）
- 为 `State` 增加 `ChannelPointerInfo(now)`：返回 `id/moved_at/reason/ok`，并复用现有指针校验与 ban 轮转逻辑（避免重复锁与重复实现）。
- `SetChannelPointer` / `ClearChannelPointer` 在写入指针时同步写入：
  - `channelPointerMovedAt = now`
  - `channelPointerReason = "manual" | "clear"`
- 为 `Scheduler` 暴露 `PinnedChannelInfo()` 供管理后台使用。

### 3.2 Admin（`internal/admin/`）
- `adminSchedulerRuntimeView` 增加 `PinnedNote`（组合好的中文提示）。
- `/admin/channels` 使用 `PinnedChannelInfo()` 填充 `PinnedNote`，并在模板徽标上通过 `title="{{.SchedulerRuntime.PinnedNote}}"` 展示。

---

## 4. 验收标准

- [√] `/admin/channels` 页头“渠道指针”徽标 hover 可看到：更新时间 + 原因（中文）。
- [√] 手动“设为指针”后，原因显示为“手动设置”。
- [√] 指针渠道进入 ban 并发生轮转后，原因显示为“因封禁轮转”。
- [√] `go test ./...` 通过。
