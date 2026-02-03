# 变更提案: 管理后台上游硬删除

## 需求背景

当前管理后台支持创建上游资源（channel/endpoint/credential/account），但缺少删除能力，导致：

1. 误配/过期的上游配置无法下线，只能保留在列表里影响维护。
2. 需要彻底移除某个 endpoint/credential/account 时缺少一键操作。

目标：在不引入复杂依赖的前提下，提供“彻底删除（硬删除）”能力，使资源从调度中移除。

## 变更内容
1. 为 channel/endpoint/credential/account 增加硬删除接口（物理删除）。
2. 管理后台列表页增加“彻底删除”按钮（带确认），并保留 status 展示字段。

## 影响范围
- **模块:**
  - `internal/store`（新增硬删除方法与按 id 查询方法）
  - `internal/admin`（新增删除 Handler）
  - `internal/server`（新增路由）
  - `internal/admin/templates`（新增删除按钮与 status 列）
- **数据:**
  - 不新增表；删除操作会物理移除记录

## 核心场景

### 需求: 上游资源下线
**模块:** Admin / Store

#### 场景: 删除 channel（级联）
- 管理员在 `/admin/channels` 点击删除
- 系统删除 channel，并级联删除其 endpoints / credentials / accounts
- 调度器不会再选择该 channel

#### 场景: 删除 endpoint/credential/account
- 在各自列表页点击删除
- 系统物理删除记录（endpoint 会级联删除其 credentials/accounts）
- 调度器不会再选择该资源

## 风险评估
- **风险:** 硬删除不可恢复，且历史审计/排错可能出现“引用 id 已不存在”的情况。
- **缓解:** 管理后台表单强制二次确认（confirm）；删除逻辑使用事务并按依赖顺序级联删除，避免残留孤儿记录。
