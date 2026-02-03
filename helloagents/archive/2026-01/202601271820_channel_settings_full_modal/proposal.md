# 变更提案: channel-settings-full-modal

## 元信息
```yaml
类型: 优化
方案类型: implementation
优先级: P1
状态: 已完成
创建: 2026-01-27
完成: 2026-01-27
```

---

## 1. 需求

### 背景
`/admin/channels` 已将行内图标入口收敛为“设置”弹窗，但仍存在两类“多余页面”：
- `/admin/channels/{channel_id}/endpoints`（端点与密钥/账号）
- `/admin/channels/{channel_id}/models` 及 `/models/{binding_id}`（渠道模型绑定）

用户希望所有配置/管理动作都在同一个“设置”小窗口内完成，不再跳转到独立页面，同时删除这些冗余页面入口。

### 目标
- 在渠道“设置”弹窗内完成：
  - `base_url`、密钥（OpenAI/Anthropic credentials）
  - Codex OAuth 账号管理（列表/新增/删除/刷新/授权）
  - 渠道模型绑定（列表/新增/编辑/删除/测试）
- 删除独立页面：端点页、渠道模型绑定页、绑定编辑页
- 保留兼容：旧 URL 访问时重定向回 `/admin/channels` 并自动打开对应渠道的“设置”弹窗（定位到对应分区）

### 验收标准
- [√] 管理后台不再提供上述独立页面入口，所有操作均可在渠道“设置”弹窗完成
- [√] 访问旧 URL 会重定向到 `/admin/channels` 并自动打开弹窗且定位到对应分区
- [√] 相关 POST 操作完成后仍回到 `/admin/channels` 并保持弹窗可继续操作（不需要用户手动找入口）
- [√] `go test ./...` 通过

---

## 2. 方案

### UI/交互
- 扩展 `internal/admin/templates/channels.html` 的“渠道设置”弹窗：
  - 新增分区：密钥、账号、模型绑定
  - 移除“端点与授权/模型绑定”等跳转按钮，改为弹窗内分区导航
  - 支持“重载后自动重新打开弹窗并定位分区”（使用 `sessionStorage` / URL 参数）

### 后端路由与兼容
- 将以下 GET 页面改为重定向：
  - `/admin/channels/{channel_id}/endpoints` → `/admin/channels?open_channel_settings={id}#keys|accounts`
  - `/admin/channels/{channel_id}/models` / `/models/{binding_id}` → `/admin/channels?open_channel_settings={id}#models`
- 将相关成功/失败跳转统一回渠道列表（带 `open_channel_settings` + hash），避免落入已删除页面。

### 影响范围
```yaml
涉及模块:
  - internal/admin: 渠道列表页模板与数据聚合、旧页面重定向
  - internal/server: Codex 刷新相关跳转目标更新
  - docs/README: 路径与操作指引更新
预计变更文件: 多
风险: 中（页面结构变大、需保证多渠道场景下 ID 唯一）
```
