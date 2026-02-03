# 轻量迭代：管理后台渠道测速“已保存”误提示

## 目标
- 点击渠道列表/模型绑定页的“测试连接”（闪电按钮）应返回明确的测试提示，而不是泛化的“已保存”。
- 避免因路由/代理/旧页面 action 异常导致误走“保存分组”等接口。

## 任务清单
- [√] 定位“已保存”来源：管理后台多处保存接口统一返回“已保存”，与测速语义冲突。
- [√] 给测速表单增加 `_intent=test_channel`，给分组表单增加 `_intent=update_channel_groups`，用于后端兜底分流。
- [√] 后端在 `POST /admin/channels` 与 `POST /admin/channels/{channel_id}` 识别 `_intent=test_channel` 并转交 `TestChannel`，避免误触发保存逻辑。
- [√] 收窄 `isChannelGroupsUpdateForm`：仅在显式意图或包含 `groups` 字段时才判定为分组更新。
- [√] 优化提示文案：分组/限额保存改为更明确的短提示；测速默认提示补充“结果已更新”。
- [√] 验证：`gofmt`、`go test ./...` 通过。

