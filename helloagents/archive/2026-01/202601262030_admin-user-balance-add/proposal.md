# 变更提案: admin-user-balance-add

## 元信息
```yaml
类型: 新功能
方案类型: implementation
优先级: P1
状态: 已完成
创建: 2026-01-26
```

---

## 1. 需求

### 背景
当前用户余额（按量计费额度）主要通过“用户充值/支付”流程增加；在客服补偿、线下转账、活动赠送等场景下，需要后台可直接为指定用户增加余额。

### 目标
- root 管理员可在管理后台为指定用户“手动增加余额（USD）”
- 管理后台可直接查看用户当前余额，便于操作前核对
- 操作成功后立即生效（用户 Dashboard / 充值页可看到余额变化）
- 关键管理操作可追溯（写入审计事件）

### 约束条件
```yaml
时间约束: 无
性能约束: 用户列表页允许一次性加载（默认用户数较少）
兼容性约束: 不引入新角色/权限体系；复用现有 /admin 权限与 CSRF/AJAX 机制
业务约束:
  - 金额单位: USD（与 user_balances.usd 一致）
  - 精度: 最多 6 位小数（与 USDScale 一致）
  - 仅支持“增加”（不提供扣减/设置余额）
```

### 验收标准
- [x] `/admin/users` 表格新增“余额(USD)”列，展示用户当前余额
- [x] `/admin/users` 提供“加余额”入口，输入 `amount_usd` 后提交即可为用户增加余额
- [x] 若用户无 `user_balances` 记录，提交时自动初始化为 0 再入账
- [x] 非法金额（空/负数/超过精度）会返回可读的错误提示
- [x] 成功操作写入 `audit_events`（包含 request_id、操作者、目标用户、增加额度与备注）

---

## 2. 方案

### 技术方案
- **后端**
  - Store 新增方法：
    - `GetUserBalancesUSD(ctx, userIDs)`：批量读取用户余额（缺失视为 0）
    - `AddUserBalanceUSD(ctx, userID, deltaUSD)`：为用户余额加值（事务内 INSERT OR IGNORE + UPDATE）
  - Admin 新增处理器：
    - `POST /admin/users/{user_id}/balance`：解析表单、校验金额、调用 Store 入账、写入 `audit_events`、返回 AJAX/重定向
- **前端（管理后台）**
  - `internal/admin/templates/users.html`：
    - 表格新增“余额(USD)”列
    - 每行新增“加余额”按钮与弹窗（显示当前余额、输入增加额度、可选备注）

### 影响范围
```yaml
涉及模块:
  - internal/store: 新增余额批量读取与加值方法
  - internal/admin: 新增用户余额加值 Handler（含审计写入）
  - internal/admin/templates: 用户管理页展示/操作入口
  - internal/server: 注册新路由
预计变更文件: 6-9
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 并发扣费/入账竞争导致余额异常 | 低 | 使用单条 `UPDATE user_balances SET usd=usd+?` 原子加值；与扣费同一行操作可安全并发 |
| 操作可追溯性不足 | 中 | 写入 `audit_events`，在 `error_message` 内记录目标用户/金额/备注（不新增表结构） |
| 管理后台列表一次性读取余额性能 | 低 | 批量 IN 查询；用户量极大时可进一步分页优化（非本次范围） |

---

## 3. 技术设计（可选）

> 涉及架构变更、API设计、数据模型变更时填写

### API设计
#### POST /admin/users/{user_id}/balance
- **请求**: `application/x-www-form-urlencoded`
  - `amount_usd`: string（必填，>0，最多 6 位小数）
  - `note`: string（可选，≤200 字符；仅用于审计记录）
- **响应**:
  - AJAX：`200 { ok: true, notice: "..." }` / `4xx { ok: false, error: "..." }`
  - 非 AJAX：重定向回 `/admin/users?msg=...` 或 `/admin/users?err=...`

### 数据模型
本变更不新增表结构；复用 `user_balances.usd` 作为余额来源，审计写入 `audit_events`。

---

## 4. 核心场景

> 执行完成后同步到对应模块文档

### 场景: 管理员为用户加余额
**模块**: internal/admin + internal/store
**条件**: root 管理员已登录后台
**行为**: 在 `/admin/users` 对目标用户点击“加余额”，输入 `amount_usd` 并提交
**结果**: `user_balances.usd` 增加对应数值；页面提示成功；审计事件记录本次操作

---

## 5. 技术决策

> 本方案涉及的技术决策，归档后成为决策的唯一完整记录

### admin-user-balance-add#D001: 余额加值走直接入账 + audit_events 记录（不新增表）
**日期**: 2026-01-26
**状态**: ✅采纳
**背景**: 需要提供可落地、改动最小的“手动加余额”能力，同时保证操作可追溯。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 新增 `user_balance_adjustments` 表 | 记录结构化、可扩展、可做后台列表 | 需要 SQLite/迁移/升级策略，改动面更大 |
| B: 直接更新 `user_balances` + 写入 `audit_events` | 无需新增表/迁移，最快落地；可满足“可追溯” | 审计明细需要编码在 `audit_events.error_message` 中，结构化较弱 |
**决策**: 选择方案 B
**理由**: 当前需求仅要求后台可手动加余额；优先以最小改动落地，并保留未来引入结构化调整表的空间。
**影响**:
  - internal/store: 新增余额加值与批量读取方法
  - internal/admin: 新增路由/Handler 与用户管理页入口
