# 实施方案: 渠道分组（channel grouping）

## 1. 数据与迁移
- 新增迁移：`internal/store/migrations/0019_channel_grouping.sql`
  - `users` 增加 `channel_group`（NOT NULL，默认 `default`）
  - `upstream_channels` 增加 `groups`（NOT NULL，默认 `default`）

## 2. store/auth/middleware
- `internal/store/models.go`
  - `User` 增加 `ChannelGroup`
  - `UpstreamChannel` 增加 `Groups`
- `internal/store/store.go`
  - TokenAuth 查询补齐 `users.channel_group`
  - 用户查询（email/username/id）补齐 `channel_group`
- `internal/auth/auth.go`：`Principal` 增加 `ChannelGroup`
- `internal/middleware/*_auth.go`：将用户分组写入 `Principal`

## 3. scheduler
- `internal/scheduler/scheduler.go`
  - `Constraints` 新增 `RequireChannelGroup`
  - `Selection` 新增 `ChannelGroups`
  - `SelectWithConstraints` 在选择渠道候选集时按分组过滤
  - 绑定命中时同时校验分组，避免绕过约束

## 4. 数据面（OpenAI 兼容）
- `internal/api/openai/handler.go`
  - 构造调度约束时写入 `RequireChannelGroup`
  - 命中旧绑定时同样校验分组

## 5. 管理后台
- `/admin/users`：用户资料编辑增加 `channel_group` 输入框（默认 default）。
- `/admin/channels`：创建渠道增加 `groups` 输入框（逗号分隔）。
- `/admin/channels/{id}/endpoints`：增加“渠道分组”卡片用于编辑 `groups`。

## 6. 使用方式（管理员）
1. 在 `/admin/channels` 创建/编辑渠道时设置 `groups`（例如：`default`、`default,vip`）。
2. 在 `/admin/users` 编辑用户资料时设置 `channel_group`（例如：`vip`）。
3. 数据面请求将自动只在匹配分组的渠道内调度。

