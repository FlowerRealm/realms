# 任务清单: channel-pointer-tree

目录: `helloagents/plan/202601282214_channel-pointer-tree/`

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
总任务: 11
已完成: 10
已跳过: 1
完成率: 91%
```

---

## 任务列表

### 1. Scheduler/State：指针与渠道环
- [√] 1.1 定义并实现 Channel Ring（树展开为环）算法（稳定 DFS + 组内稳定排序）
- [√] 1.2 在 `State` 增加 `pointerChannelID + pointerRing(+index)` 组合态与原子推进方法
- [√] 1.3 在 `Scheduler.Report` 中：当指针渠道 ban 生效时自动推进到下一个（含环形回绕与跳过 ban）

### 2. GroupRouter：以指针为唯一入口
- [√] 2.1 `GroupRouter` 在指针开启时按 “ring 从指针位置开始遍历一圈” 的顺序 failover
- [√] 2.2 指针自愈：当指针不在 ring/不可用时修正到 ring 的合理位置（并记录 reason）

### 3. 兼容现有策略
- [√] 3.1 指针开启时旁路/覆盖会话粘性绑定与亲和（确保“唯一标定”成立）
- [√] 3.2 指针关闭时保持现有选择策略不变（回归测试）

### 4. 管理后台（UI/API）
- [√] 4.1 `/admin/channels` 页头/行内标记展示当前指针；按钮语义明确为“设为指针”
- [-] 4.2 （可选）提供“清除指针，恢复自动调度”入口

### 5. 验证
- [√] 5.1 单元测试：覆盖“ban 触发轮转 + 回绕 + 跳过 ban + 不被 binding 拉走”
- [√] 5.2 运行测试：`go test ./...`
