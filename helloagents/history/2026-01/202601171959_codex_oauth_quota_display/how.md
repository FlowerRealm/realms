# 技术设计: Codex OAuth 用量展示口径修复（5 小时 / 每周）

## 技术方案

### 核心技术
- Go `html/template` 渲染：`internal/admin/templates/endpoints.html`
- 管理端视图拼装：`internal/admin/server.go`
- quota 数据来源：`internal/codexoauth/quota.go`（已解析 `used_percent` + `reset_at` + credits）

### 实现要点
- 在 admin 层引入两个额度上限（Team 口径）：
  - 5 小时额度：`$6 / 5h`
  - 每周额度：`$20 / week`
- 基于 `used_percent` 计算剩余金额（USD，估算）：
  - `leftPercent = clamp(100-usedPercent, 0..100)`
  - `remainingUSD = capUSD * leftPercent / 100`
- 复用现有 `reset_at`，在 UI 中直接展示（RFC3339，UTC）；若缺失则仅展示剩余金额或 `-`
- 模板层改名与补全展示：
  - “主额度窗口”→“5 小时额度窗口”
  - “次额度窗口”→“周限额与代码审查额度窗口”
  - 在进度条下新增一行细节文本：`剩余 $X/$Cap · 重置 <time>`

### 降级策略
- `QuotaError` 非空：只显示错误，不展示额度明细（避免误导）
- `used_percent` 缺失：显示 `-`
- `PlanType` 非 Team：默认仍展示百分比+重置时间；金额展示可通过简单判定开关控制（优先 KISS：先实现“可读且不误导”，再细化）

## 安全与性能
- **安全:** 仅展示与格式化，不引入新权限；不输出 token 等敏感信息
- **性能:** 仅字符串计算，无额外 IO/网络请求

## 测试与部署
- **测试:** 为金额计算与展示格式化增加单元测试（边界：nil、超范围 percent、无 reset）
- **部署:** 无数据库变更；重新构建并发布即可

