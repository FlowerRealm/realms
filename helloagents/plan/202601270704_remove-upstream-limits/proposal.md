# 变更提案: remove_upstream_limits

## 元信息
```yaml
类型: 变更/重构
方案类型: implementation
优先级: P1
状态: ✅完成
创建: 2026-01-26
```

---

## 1. 需求

### 背景
渠道管理此前支持配置 `Sessions/RPM/TPM` 的“限额”能力（渠道默认 + 密钥/账号级），并在调度选择时按超限跳过候选。现在希望渠道管理中不再存在这类限制，并将相关数据库字段与功能代码彻底移除。

### 目标
- 管理后台不再展示/编辑任何 `Sessions/RPM/TPM` 限额
- 调度不再因 `Sessions/RPM/TPM` 超限过滤候选（但保留运行态 RPM/TPM/Sessions 统计用于观测与负载均衡）
- 数据库中删除相关字段（MySQL 迁移 + SQLite 初始化 schema 同步）

### 约束条件
```yaml
时间约束: 无
性能约束: 不降低现有调度性能
兼容性约束: MySQL 通过可重入迁移删除列；SQLite 通过初始化 schema 更新（存量 SQLite DB 需重建或自行处理列删除）
业务约束: 仅移除限额能力，不移除 RPM/TPM/Sessions 运行态统计
```

### 验收标准
- [ ] 管理后台 `/admin/channels` 与 `/admin/channels/{id}/endpoints` 不再出现“限额”相关 UI
- [ ] 服务端不再注册 `/admin/*/limits` 路由，且代码中无 `limit_sessions/limit_rpm/limit_tpm` 字段读写
- [ ] MySQL 迁移可重入：在不同历史 schema 下执行均不会失败（列存在则删，不存在则跳过）
- [ ] `go test ./...` 全部通过

---

## 2. 方案

### 技术方案
1) 删除 store/models 中的 `LimitSessions/LimitRPM/LimitTPM` 字段，并清理所有 SQL 读写与导出/导入配置字段。
2) 删除管理后台限额相关 handler 与模板（Channels/Endpoints 页面移除“限额”展示与编辑）。
3) 调度器移除按限额过滤候选的逻辑，保留 RPM 最低优先等既有选择策略与运行态统计。
4) 数据库层通过新迁移删除相关列，并将旧限额迁移改为占位 no-op，避免新初始化继续引入该字段。

### 影响范围
```yaml
涉及模块:
  - internal/store: 移除字段与 SQL
  - internal/scheduler: 移除限额过滤
  - internal/admin + templates: 移除限额配置 UI
  - internal/server: 移除 limits 路由注册
  - internal/store/migrations + schema_sqlite.sql: 删除列/占位迁移
  - helloagents/wiki + CHANGELOG: 文档同步
预计变更文件: 10+
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| MySQL 删除列造成配置丢失 | 中 | 属于预期变更；迁移可重入且仅删除相关列 |
| 存量 SQLite DB 仍有旧列 | 低 | SQLite 初始化 schema 已去除字段；存量 DB 需重建或手工删除列 |
| UI/路由引用残留导致 404/模板错误 | 低 | 同步移除模板引用与路由注册，并通过测试验证编译与用例 |

---

## 4. 核心场景

> 执行完成后同步到对应模块文档

### 场景: 渠道管理不再配置限额
**模块**: Admin SSR / Scheduler / Store
**条件**: 管理员进入 `/admin/channels` 与 `/admin/channels/{id}/endpoints`
**行为**: 页面不展示“限额”字段；调度选择不再因限额跳过候选
**结果**: 仅保留运行态 RPM/TPM/Sessions 观测；系统行为不受限额配置影响

---

## 5. 技术决策

> 本方案涉及的技术决策，归档后成为决策的唯一完整记录

### remove_upstream_limits#D001: 通过“新增删除列迁移 + 旧迁移占位”完成清理
**日期**: 2026-01-26
**状态**: ✅采纳
**背景**: 需要“删干净”，同时避免历史 schema/迁移顺序导致初始化或升级失败。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 仅删代码，保留数据库列 | 改动小 | DB 仍残留字段，不符合“删干净” |
| B: 新增迁移删除列（可重入），并将旧限额迁移改为占位 | 新/旧 DB 均可收敛到无字段；新初始化不再引入限额 | 需要同步调整后续迁移列位置与 SQLite schema |
**决策**: 选择方案 B
**理由**: 同时满足“彻底移除功能”和“数据库字段删除”，并尽量兼容不同历史 schema。
**影响**: store/migrations、store/schema_sqlite、store/scheduler/admin 相关代码与文档。
