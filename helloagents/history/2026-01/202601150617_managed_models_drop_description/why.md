# 变更提案: 移除模型 description 字段（managed_models）

## 需求背景

当前 `managed_models` 表与 Store 层模型包含 `description` 字段，但该字段对核心功能（模型白名单、模型绑定、定价、调度与计费）没有必要性：

- 管理后台当前已不提供 description 的录入/编辑入口
- Web 控制台模型列表仅用于展示，可在不依赖 description 的情况下保持信息清晰
- 保留无主的可选文本字段会增加数据维护成本与 API/模板/SQL 的耦合点

因此计划移除 `managed_models.description` 以及运行态相关字段，保持数据模型最小化。

## 变更内容

1. 数据库：删除 `managed_models.description` 列（新增迁移文件）
2. Store：移除 `store.ManagedModel` / `ManagedModelCreate` / `ManagedModelUpdate` 的 `Description` 字段，并更新相关 SQL
3. Web 控制台：模型列表不再展示 Description 列
4. 管理后台：移除与 description 相关的表单读取/写入逻辑（如仍存在）

## 影响范围

- **模块:**
  - `internal/store`（数据结构 + SQL）
  - `internal/web`（模型列表页面）
  - `internal/admin`（模型管理 handler/view）
- **文件:**
  - `internal/store/models.go`
  - `internal/store/managed_models.go`
  - `internal/store/migrations/*.sql`
  - `internal/web/server.go`
  - `internal/web/templates/models.html`
  - `internal/admin/models.go`
- **API:**
  - `GET /v1/models` 不受影响（当前未包含 description）
- **数据:**
  - `managed_models.description` 列删除（历史数据将被丢弃）

## 核心场景

### 需求: 移除模型 description 字段
**模块:** store/web/admin

#### 场景: 数据库迁移
在升级部署执行迁移后：
- `managed_models` 不再包含 `description` 列
- 服务启动迁移可正常通过

#### 场景: 模型列表展示
Web 控制台 `/models`：
- 模型列表可正常渲染（不依赖 description）
- 表头与行内容不再出现 Description 列

#### 场景: 管理后台模型管理
管理后台 `/admin/models` 与编辑页：
- 新建/编辑模型不依赖 description 字段
- Create/Update 流程不应因缺少 description 而失败

## 风险评估

- **风险:** 删除列属于不可逆变更，若有人依赖 description 内容将丢失
- **缓解:** 升级前按需备份；迁移与代码同时发布，避免运行时 SQL/struct 不一致

