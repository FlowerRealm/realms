# 任务清单: 渠道分组下拉选择 + 分组管理页

目录: `helloagents/plan/202601152124_channel_groups_ui/`

---

## 1. 数据与 store
- [√] 1.1 新增迁移：创建 `channel_groups` 并插入 `default`。
- [√] 1.2 store：新增 `ChannelGroup` 模型与 CRUD（含引用计数与删除保护）。

## 2. 管理后台页面
- [√] 2.1 新增 `/admin/channel-groups` 页面与路由（左侧菜单入口）。
- [√] 2.2 支持新建/编辑/删除（default 分组禁止禁用/删除；引用时拒绝删除）。

## 3. 下拉选择改造
- [√] 3.1 `/admin/users`：`channel_group` 改为下拉单选。
- [√] 3.2 `/admin/channels`：创建渠道 `groups` 改为下拉多选。
- [√] 3.3 `/admin/channels/{id}/endpoints`：编辑渠道 `groups` 改为下拉多选。

## 4. 校验与兼容
- [√] 4.1 后端校验：仅允许选择存在且启用的分组（未变更时允许保留旧值以兼容历史数据）。
- [√] 4.2 UI 兼容：已使用但未注册的分组回显为“禁用/未注册”选项。

## 5. 文档与测试
- [√] 5.1 执行 `go test ./...`。
- [√] 5.2 更新知识库：`helloagents/wiki/data.md`、`helloagents/wiki/modules/realms.md`、`helloagents/CHANGELOG.md`。
- [√] 5.3 迁移方案包至 `helloagents/history/2026-01/` 并更新 `helloagents/history/index.md`。

