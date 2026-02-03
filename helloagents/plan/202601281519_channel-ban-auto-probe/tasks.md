# 任务清单: 渠道封禁到期自动探测（半开）+ 封禁时长上限

目录: `helloagents/plan/202601281519_channel-ban-auto-probe/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态

```yaml
总任务: 14
已完成: 14
完成率: 100%
```

---

## 任务列表

### 1. Scheduler 状态与策略

- [√] 1.1 增加 channel probe 运行态（due/claim）与清理方法（`internal/scheduler/state.go`）
- [√] 1.2 封禁到期时自动标记 probe due（`State.IsChannelBanned`）
- [√] 1.3 渠道封禁时长上限收敛到 10 分钟（`State.BanChannel` clamp 到 `now+10m`）
- [√] 1.4 封禁到期 probe 排序优先级：forced 后、promotion 前（`internal/scheduler/group_router.go`）
- [√] 1.5 非组路由路径的 probe 排序优先级（`internal/scheduler/scheduler.go`）
- [√] 1.6 探测单飞：probe channel 被占用时跳过，避免并发探测风暴（`Scheduler.SelectWithConstraints`）
- [√] 1.7 探测完成后清理 probe 状态；成功时重置渠道失败分值并回归使用（`Scheduler.Report`）

### 2. 测试与验证

- [√] 2.1 新增单测覆盖：ban 上限、到期 probe、probe 单飞、success 重置 failScore（`internal/scheduler/scheduler_test.go`）
- [√] 2.2 运行全量测试：`go test ./...`

### 3. 无流量自动探测（后台）

- [√] 3.1 增加 probe due 列表能力（`internal/scheduler/state.go`）
- [√] 3.2 Scheduler 暴露 probe/ban 操作封装（`internal/scheduler/scheduler.go`）
- [√] 3.3 复用管理后台渠道测试：导出 `admin.RunChannelTest`（`internal/admin/channel_health.go`）
- [√] 3.4 App 启动后台探测 loop（`internal/server/app.go`）
- [√] 3.5 新增单测覆盖：probe due 列表排序 + claim 过滤（`internal/scheduler/scheduler_test.go`）

---

## 验收标准

- [√] 任意 channel 的封禁时长不超过 10 分钟（以 `now` 为基准）。
- [√] 封禁到期后 channel 会被自动“半开探测”一次；无流量场景下后台自动触发测试；成功后可回归正常调度顺序。
- [√] 并发请求场景下不会出现 probe channel 被持续顶到第一位导致探测风暴。
- [√] 现有调度/路由/失败切换行为不被破坏（相关测试通过）。
