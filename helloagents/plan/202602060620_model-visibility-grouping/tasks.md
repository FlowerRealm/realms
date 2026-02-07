# 任务清单：模型可见性分组与归属方二级展示（model-visibility-grouping）

目录: `helloagents/plan/202602060620_model-visibility-grouping/`

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
总任务: 20
已完成: 18
完成率: 90%
```

---

## 任务列表

### 1. Figma 设计输入与确认

- [?] 1.1 获取模型展示页对应 Figma 链接（Frame/Node URL）
  - 验证: 链接可提取 node_id，且覆盖“分组 + 归属方二级分层”视觉稿

- [?] 1.2 按技能流程抓取设计上下文：`get_design_context` → `get_screenshot`（必要时 `get_metadata`）
  - 依赖: 1.1
  - 产出: 设计约束清单（间距、标题层级、折叠交互）

### 2. 数据结构与迁移

- [√] 2.1 在 `internal/store/schema_sqlite.sql` 的 `managed_models` 表增加 `group_name` 与索引
  - 验证: 新库初始化后字段存在，默认值 `default`

- [√] 2.2 新增 MySQL migration（建议 `0051_managed_models_group_name.sql`）回填历史数据
  - 验证: 旧库升级后 `group_name` 非空且值为 `default`

- [√] 2.3 更新 `internal/store/models.go` 的 `ManagedModel`、Create/Update 输入结构
  - 依赖: 2.1, 2.2

### 3. Store 查询与可见性规则

- [√] 3.1 更新 `internal/store/managed_models.go` 所有 SELECT/INSERT/UPDATE 字段映射，纳入 `group_name`
  - 验证: CRUD 后读取字段正确

- [√] 3.2 调整 `ListEnabledManagedModelsWithBindingsForGroups`：按用户组匹配 `m.group_name`
  - 验证: 用户仅得到 group 交集内模型

- [√] 3.3 保留 channel 绑定可用性约束，确保展示模型可被真实调度
  - 验证: 仅配置模型但未绑定渠道的条目不会返回

### 4. API 扩展（router）

- [√] 4.1 更新 `router/models_api_routes.go` 的 `managedModelView`/`userManagedModelView` 输出 `group_name`
  - 验证: `/api/models/`、`/api/user/models/detail` 响应含字段

- [√] 4.2 更新 admin create/update 请求体支持 `group_name`（空值归一为 `default`）
  - 验证: 提交表单可保存并回读

- [√] 4.3 检查 `internal/store/admin_export_import.go` 对 `managed_models` 导入导出字段兼容性
  - 验证: 导出包含 `group_name`，导入可回放

### 5. Web 管理端改造

- [√] 5.1 更新 `web/src/api/models.ts` 类型与请求参数，新增 `group_name`
  - 验证: TS 编译通过

- [√] 5.2 在 `web/src/pages/admin/ModelsAdminPage.tsx` 接入分组选择器（复用 `listAdminChannelGroups`）
  - 依赖: 4.2, 5.1
  - 验证: 新增/编辑可选分组，默认 `default`

- [√] 5.3 管理列表新增“模型分组”列与徽标展示
  - 验证: 列表中可直观看到分组

### 6. Web 用户端展示改造

- [√] 6.1 更新 `web/src/pages/ModelsPage.tsx`：按 `group_name` 一级分块展示
  - 依赖: 4.1
  - 验证: 页面不再单表平铺全部模型

- [√] 6.2 在组内按 `owned_by` 二级分段（未设置归属方归并为 `unknown`）
  - 验证: 同组下不同归属方可视化隔离

- [√] 6.3 样式与交互按参考实现核对（标题层级、组间距、视觉密度）
  - 依赖: 1.2（若提供 Figma）
  - 验证: 左侧分组选择 + 右侧模型单行列表，视觉密度与 `new-api` 风格对齐

### 7. 验证与知识库同步

- [√] 7.1 新增/更新后端测试（重点：组过滤、默认分组回填、接口字段）
  - 验证: `go test ./...` 通过

- [√] 7.2 前端构建验证
  - 验证: `cd web && npm run build` 通过

- [√] 7.3 更新 `helloagents/CHANGELOG.md` 记录本次实施结果（完成后）
  - 验证: 变更条目可追溯到方案包

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 1.1 | [?] | 当前会话尚未提供 Figma URL；已确认 Figma MCP 登录可用（`whoami` 成功） |
| 1.2 | [?] | 因缺少 node 链接，`get_design_context/get_screenshot` 暂未执行 |
| 2.x~6.2 | [√] | 后端/前端主链路已完成：group_name 字段、可见性过滤、管理端分组选择、用户端分组展示 |
| 7.1 | [√] | `go test ./...` 通过 |
| 7.2 | [√] | `cd web && npm run build` 通过 |
| 6.3 | [√] | 按用户反馈参考 `new-api` 重做样式：左侧分组导航、右侧单行模型表、紧凑视觉密度，并使用 Playwright 登录实测与截图验证 |
| 6.2 | [√] | 根据新需求调整为“用户组下按 `owned_by` 二级分组”展示：右侧先显示归属方区块，再列模型单行；Playwright 截图复核 |
| 6.1 | [√] | 按用户反馈统一展示语义：左侧与主标题使用归属方显示名（不展示 `default` 等原始 `group_name`） |
| 3.2 | [√] | 增补 SQLite 回归覆盖：多组查询（vip+default）不泄露 foreign group（admin）模型 |
