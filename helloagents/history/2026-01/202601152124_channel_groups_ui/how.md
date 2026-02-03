# 实施方案: 渠道分组下拉选择 + 分组管理页

## 1. 数据层
- 新增表：`channel_groups`
  - `name` 唯一
  - `status` 启用/禁用
  - `description` 可选
- 迁移中自动插入 `default` 分组。

## 2. store 层
- 增加 `ChannelGroup` 模型与 CRUD：
  - 列表/按 id/name 查询
  - 创建/编辑（描述+状态）
  - 删除（仅当未被 users/channel 引用）
  - 引用计数（`users.channel_group`、`FIND_IN_SET(..., upstream_channels.groups)`）

## 3. 管理后台 UI/路由
- 新增页面：`GET /admin/channel-groups`
  - 新建分组（modal）
  - 编辑分组（modal）
  - 删除分组（引用时拒绝）
- 左侧菜单新增入口“渠道分组”。

## 4. 下拉选择改造
- `/admin/users` 用户资料编辑：
  - `channel_group` 改为下拉单选（来源：`channel_groups`）
- `/admin/channels` 创建渠道：
  - `groups` 改为下拉多选（来源：`channel_groups`）
- `/admin/channels/{id}/endpoints` 渠道分组编辑：
  - `groups` 改为下拉多选（来源：`channel_groups`）

## 5. 兼容策略
- 若用户/渠道已使用某个分组，但该分组未存在于 `channel_groups`：
  - 页面仍会将其作为“未注册”选项回显（避免误删）
  - 但禁止新增/修改到未注册或禁用分组（保持一致性）

