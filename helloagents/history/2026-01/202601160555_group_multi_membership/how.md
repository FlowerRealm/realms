# 技术设计: Group（用户多组）与组内渠道约束

## 技术方案

### 核心技术
- Go `net/http`
- MySQL（内置迁移 `internal/store/migrations/*.sql`）

### 实现要点
1. **用户组集合的 SSOT**
   - 数据库存储使用 `user_groups`（多对多）
   - 运行时从 DB 查询用户所属组集合（必要时可用 `GROUP_CONCAT` 聚合成 CSV 再 split）
2. **调度约束从“单组”升级为“允许组集合”**
   - `scheduler.Constraints` 增加 `AllowGroups`（集合语义）
   - 选择与绑定校验统一使用“渠道组与允许组集合是否有交集”
3. **订阅套餐组字段迁移**
   - 将 `subscription_plans.channel_group` 迁移为 `subscription_plans.group_name`
   - Web 侧展示与购买均按用户组集合过滤与校验
4. **管理台用户组多选**
   - 用户编辑表单改为 checkbox 多选
   - 服务端强制包含 `default`，避免“无组用户”导致调度不可用

## 数据模型

### 新增表: `user_groups`

- `user_id` BIGINT NOT NULL
- `group_name` VARCHAR(64) NOT NULL
- `created_at` DATETIME NOT NULL
- UNIQUE KEY (`user_id`, `group_name`)
- 索引: `idx_user_groups_group_name`（按组统计/删除校验使用）

### 迁移策略（新增迁移文件）
1. 创建 `user_groups`（若不存在）
2. 回填：
   - 为所有用户写入 `default`
   - 若存在 `users.channel_group`，将其回填到 `user_groups`
3. 移除 `users.channel_group`（条件判断可重入）
4. `subscription_plans.channel_group` → `group_name`（回填后移除旧列）

## API/业务行为设计

### 数据面调度（/v1/*）
- 约束来源：
  - 基础约束：`principal.Groups`（用户组集合）
  - 配额约束：若本次请求选择了订阅套餐，则将允许组集合收敛到该套餐组（并校验该组属于用户组集合）

### Web 订阅购买
- 列表：仅返回 `plan.group_name ∈ principal.Groups` 的套餐
- 购买：服务端校验套餐组是否属于用户组集合，防止绕过前端直接 POST

## 安全与性能

- **安全:**
  - 管理台保存用户组时校验组名合法且组存在/启用
  - 购买订阅时服务端校验用户是否属于该套餐组
  - 数据面调度永远基于 token/session 对应用户的组集合过滤渠道
- **性能:**
  - 用户组数量通常很小，允许用 `GROUP_CONCAT` 或二次查询取组集合
  - 调度过滤发生在候选渠道列表上（通常数量也有限）

## 测试与部署

- **测试:** `go test ./...`
  - 覆盖调度约束的交集判定与绑定约束
  - 覆盖 openai handler 中约束匹配逻辑（防止绑定绕过组限制）
- **部署:**
  - 先部署代码，再由应用启动时自动执行迁移
  - 迁移完成后，管理台可见用户组从单选变为多选

