# 变更提案：模型可见性分组与归属方二级展示（model-visibility-grouping）

## 元信息
```yaml
类型: 优化 + 新功能
方案类型: implementation
优先级: P1
状态: 待评审
创建: 2026-02-06
```

---

## 1. 需求

### 背景
当前模型可见性与展示存在两个痛点：

1. **用户端展示拥挤**：`web/src/pages/ModelsPage.tsx` 当前按平铺表格展示所有可用模型，仅用 `owned_by` 徽标展示归属方，无法按业务分组隔离。
2. **模型缺少显式分组属性**：`managed_models` 当前有 `owned_by` 但无 `group_name`，模型“属于哪个用户组”的语义主要由渠道分组间接决定，模型管理侧无法直接分组选中。

### 目标
1. 为模型增加可管理的分组属性（`group_name`），支持在管理侧选择。
2. 用户端仅可见“属于用户组”的模型（基于用户 `groups` 与模型 `group_name` 交集）。
3. 模型展示页按“模型分组 → 归属方（owned_by）”二级分隔，降低视觉拥挤。
4. 保持现有接口兼容：`/api/user/models` 仍返回字符串列表，新增字段主要体现在 detail/admin 接口。

### 约束条件
```yaml
时间约束: 本次先完成后端 + Web SPA（不改 SSR 旧页面）
性能约束: 用户模型查询仍需走 SQL 层过滤，避免前端大列表二次筛
兼容性约束: 旧数据必须可用（历史模型默认映射到 default 组）
业务约束: 分组来源复用现有 channel_groups/user_groups 命名体系
设计约束: Figma 走 get_design_context → get_screenshot 规范流程，需用户提供节点链接
```

### 验收标准
- [ ] 管理侧新增/编辑模型可设置 `group_name`，且默认值为 `default`。
- [ ] `/api/user/models/detail` 返回的模型仅来自用户组可见范围，并包含 `group_name` 字段。
- [ ] 用户模型页按分组块展示，组内按 `owned_by` 二级分段，不再单表平铺。
- [ ] 无分组或历史数据场景可正常回退到 `default` 组，不出现“空白模型页”误伤。
- [ ] `go test ./...` 与 `cd web && npm run build` 通过。

---

## 2. 方案

### 技术方案
采用“**模型单分组字段 + 前端分层展示**”的最小可行方案：

1. **数据模型层**
   - 为 `managed_models` 增加 `group_name`（非空，默认 `default`）。
   - 新增索引（`status`, `group_name`）优化用户可见性查询。
   - SQLite 基线 schema 与 MySQL migration 同步更新。

2. **查询与可见性层**
   - 在 `ListEnabledManagedModelsWithBindingsForGroups` 中加入 `m.group_name` 过滤。
   - 维持“必须有可用 channel 绑定”约束，确保展示模型可被真实路由。
   - 当用户无分组数据时，仍按 `default` 组兜底。

3. **API 层**
   - 扩展 `managedModelView` / `userManagedModelView` 返回 `group_name`。
   - 扩展 admin create/update 请求体支持 `group_name`。

4. **管理端 UI（ModelsAdminPage）**
   - 新增 `group_name` 选择器（优先复用 `listAdminChannelGroups()` 获取分组选项）。
   - 列表新增“模型分组”列，支持快速识别。

5. **用户端 UI（ModelsPage）**
   - 前端按 `group_name` 分块渲染，每块内再按 `owned_by` 分段。
   - 保留当前价格/状态信息；聚焦结构拆分，不引入复杂交互控件。

6. **Figma 对齐流程（本需求指定）**
   - 待用户提供 Figma 节点链接后，按 `figma` 技能规范执行：
     1) `get_design_context`
     2) （必要时）`get_metadata`
     3) `get_screenshot`
   - 以截图校准分组区块样式与层级间距，再落地到现有 Bootstrap 组件体系。

### 影响范围
```yaml
涉及模块:
  - internal/store: managed_models 结构、查询条件、增改字段
  - internal/store/migrations + schema_sqlite.sql: 新增 group_name 字段与索引
  - router/models_api_routes.go: admin/user 模型接口字段扩展
  - web/src/api/models.ts: TS 类型与请求结构扩展
  - web/src/pages/admin/ModelsAdminPage.tsx: 分组选择与列表展示
  - web/src/pages/ModelsPage.tsx: 分组/归属方二级渲染
  - tests 与 router/*_test.go: 新增或补充分组可见性测试
预计变更文件: 10~14
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 历史模型无 group_name 导致不可见 | 中 | migration 默认回填 `default`，查询层空值归并到 `default` |
| 模型组与渠道组配置不一致导致“看得到但不可用”或“可用但看不到” | 中 | 明确查询语义为“双重约束”（模型组+渠道组），并在管理页提示 |
| Figma 资源未提供导致 UI 样式确认延期 | 低 | 先按现有设计系统实现结构，待节点链接后做样式对齐微调 |
| 字段扩展影响导入导出 | 中 | 同步更新 `admin_export_import.go` 的 managed_models 序列化结构 |

---

## 3. 技术设计

### 架构设计
```mermaid
flowchart LR
    A[admin/models 配置 group_name] --> B[managed_models.group_name]
    B --> C[Store: 按用户组过滤模型]
    C --> D[/api/user/models/detail 返回 group_name + owned_by]
    D --> E[ModelsPage 按 group_name 分块]
    E --> F[组内按 owned_by 分段]
```

### API设计
#### `GET /api/user/models/detail`
- **新增响应字段**: `group_name`
- **语义**: 仅返回用户可见组内、且有可用渠道绑定的启用模型

#### `POST /api/models/`（admin）
- **新增请求字段**: `group_name`（可选，空值归一到 `default`）

#### `PUT /api/models/`（admin）
- **新增请求字段**: `group_name`（与 `public_id`、`owned_by` 同级）

### 数据模型
| 字段 | 类型 | 说明 |
|------|------|------|
| `managed_models.group_name` | VARCHAR(64)/TEXT | 模型所属分组，默认 `default` |
| `idx_managed_models_status_group` | INDEX | 优化状态+分组查询 |

---

## 4. 核心场景

### 场景: 管理员为模型设置分组
**模块**: admin models
**条件**: root 登录且存在 channel groups
**行为**: 在新增/编辑模型表单选择 `group_name`
**结果**: 模型保存成功并带分组属性

### 场景: 普通用户查看模型页
**模块**: user models page
**条件**: 用户 `groups` 已配置
**行为**: 请求 `/api/user/models/detail`
**结果**: 页面仅显示该用户组可见模型，按“分组→归属方”展示

### 场景: 历史数据升级
**模块**: migration + store
**条件**: 数据库存在旧版 `managed_models` 数据
**行为**: 执行 migration 回填 `group_name='default'`
**结果**: 升级后不丢模型可见性

---

## 5. 技术决策

### model-visibility-grouping#D001: 模型分组采用单字段 `group_name`（非多对多表）
**日期**: 2026-02-06
**状态**: ✅采纳
**背景**: 当前需求核心是“隔开展示 + 按用户组可见”，不要求一个模型同时归属多个用户组。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: `managed_models.group_name` 单字段 | 实现快、迁移简单、查询清晰 | 一个模型仅能归属一个组 |
| B: `managed_model_groups` 关联表 | 灵活支持多组 | 查询与维护复杂度显著上升 |
**决策**: 选择方案 A
**理由**: 先满足当前明确诉求，避免过度设计；后续如出现“多组同模型”真实需求，再升级关系模型。
**影响**: store 查询、admin API、web admin/user 页面都需扩展 `group_name` 字段。

### model-visibility-grouping#D002: 用户端采用“服务端过滤 + 前端分层渲染”
**日期**: 2026-02-06
**状态**: ✅采纳
**背景**: 需要保证安全可见性且提升阅读性。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 服务端过滤 + 前端仅展示 | 权限边界清晰，不泄露无权模型 | 后端改造更多 |
| B: 后端全量返回，前端筛选 | 前端实现快 | 存在越权泄露风险 |
**决策**: 选择方案 A
**理由**: 可见性必须在服务端保证；前端只负责结构化展示。
**影响**: `ListEnabledManagedModelsWithBindingsForGroups` 与 `/api/user/models/detail` 需要同步调整。
