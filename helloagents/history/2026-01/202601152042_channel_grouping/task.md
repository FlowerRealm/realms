# 任务清单: 渠道分组（channel grouping）

目录: `helloagents/plan/202601152042_channel_grouping/`

---

## 1. 数据与 store 层
- [√] 1.1 新增迁移：`users.channel_group`、`upstream_channels.groups`（默认 `default`）。
- [√] 1.2 扩展 `internal/store/models.go`：补齐 `User.ChannelGroup` 与 `UpstreamChannel.Groups`。
- [√] 1.3 扩展 TokenAuth：鉴权时读取并返回用户 `channel_group`。
- [√] 1.4 扩展用户查询/列表：补齐 `channel_group` 字段读取。
- [√] 1.5 新增写接口：`SetUserChannelGroup`、`SetUpstreamChannelGroups`。

## 2. 调度器与数据面
- [√] 2.1 `scheduler.Constraints` 增加 `RequireChannelGroup`。
- [√] 2.2 `scheduler.Selection` 增加 `ChannelGroups`，并在绑定命中校验分组。
- [√] 2.3 OpenAI handler 将 `Principal.ChannelGroup` 下发为调度约束，避免粘性绑定绕过。

## 3. 管理后台
- [√] 3.1 `/admin/users` 用户资料编辑增加 `channel_group` 配置入口。
- [√] 3.2 `/admin/channels` 创建渠道支持设置 `groups`。
- [√] 3.3 `/admin/channels/{id}/endpoints` 支持编辑 `groups`。

## 4. 文档与知识库
- [√] 4.1 更新 `helloagents/CHANGELOG.md`（Unreleased 记录本次新增）。
- [√] 4.2 更新 `helloagents/wiki/data.md`：补齐 `channel_group/groups` 数据模型说明。
- [√] 4.3 更新 `helloagents/wiki/modules/realms.md`：补齐渠道分组说明与变更历史条目。
- [√] 4.4 迁移方案包至 `helloagents/history/2026-01/` 并更新 `helloagents/history/index.md`。

## 5. 测试
- [√] 5.1 执行 `go test ./...` 并确保通过。

