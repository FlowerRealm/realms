# 变更提案: 移除模型 updated_at 字段（managed_models）

## 需求背景

当前 `managed_models` 表包含 `updated_at` 字段，且 Store 与管理后台会读取并展示该字段。但对模型目录来说：

- 模型是否可用由 `status` + 绑定关系（`channel_models` / `upstream_channels`）决定
- 定价与元信息维护是管理动作，运行态并不依赖 `updated_at`
- 管理后台展示 “Updated” 会促使人为关注无意义的时间戳，增加维护噪音

因此计划移除 `managed_models.updated_at`，让数据模型更精简，避免无必要字段在 SQL/结构体/模板中扩散。

## 变更内容

1. 数据库：删除 `managed_models.updated_at` 列（新增迁移文件）
2. Store：移除 `store.ManagedModel.UpdatedAt`，并更新 managed_models 的 SELECT/INSERT/UPDATE
3. 管理后台：模型列表不再展示 Updated 列（以及 view 结构体相关字段）

## 影响范围

- **模块:**
  - `internal/store`（数据结构 + SQL）
  - `internal/admin`（模型管理展示）
- **文件:**
  - `internal/store/migrations/*.sql`
  - `internal/store/models.go`
  - `internal/store/managed_models.go`
  - `internal/admin/models.go`
  - `internal/admin/templates/models.html`
- **API:**
  - `GET /v1/models` 不受影响（不包含 updated 字段）
- **数据:**
  - `managed_models.updated_at` 列删除（历史更新时间将丢弃）

## 核心场景

### 需求: 移除模型 updated 字段
**模块:** store/admin

#### 场景: 数据库迁移
升级部署执行迁移后：
- `managed_models` 不再包含 `updated_at` 列
- 服务启动迁移可正常通过

#### 场景: 管理后台模型列表
管理后台 `/admin/models`：
- 模型列表可正常渲染
- 表头与行内容不再出现 Updated 列

#### 场景: 管理后台新增/编辑模型
管理后台新增/编辑模型：
- Create/Update 不再写入 `updated_at`
- 不因缺少 `updated_at` 字段而失败

## 风险评估

- **风险:** 删除列属于不可逆变更，丢失历史更新时间
- **缓解:** 迁移与代码同步发布；如确实需要审计时间戳，应使用独立审计表而非业务表冗余字段

