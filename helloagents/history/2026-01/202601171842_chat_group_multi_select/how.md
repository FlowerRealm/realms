# 技术设计: 对话分组支持多选

## 技术方案

### 核心技术
- Go（`net/http`）
- MySQL（复用 `app_settings`、`channel_groups`、`upstream_channels.groups`）

### 实现要点
1. **存储格式：** 复用 `app_settings.chat_group_name`，允许保存 `g1,g2,g3` 的 CSV 形式（兼容旧单值）。
2. **运行态解析：**
   - `internal/store` 新增 `ResolveChatGroupNames(ctx)`：读取 CSV → 过滤无效/重复 → 查询 `channel_groups` → 仅返回 `status=1` 的分组名列表。
   - 若最终为空：对外表现为 `sql.ErrNoRows` 或 `ErrChatGroupDisabled`（调用方均回退到用户分组逻辑）。
3. **对话请求约束：**
   - OpenAI 兼容 handler 在 `X-Realms-Chat: 1` 且对话分组集合存在时，将 `cons.AllowGroups` 设为集合（不再是单元素）。
   - 模型可用性校验按“并集”判断：只要存在任一绑定渠道属于集合即可通过。
4. **对话模型列表：**
   - `/api/chat/models` 在对话分组集合存在时，调用 `ListEnabledManagedModelsWithBindingsForGroups(ctx, groups)`，按并集返回模型。
5. **管理后台多选：**
   - `/admin/channel-groups` 增加“对话分组设置”弹窗：checkbox 多选保存到 `app_settings.chat_group_name`（CSV）。
   - 兼容保留原有单个分组的 set/unset 端点，但语义调整为“加入/移出集合”（更符合多选）。

## 架构设计
无新增模块；仅在现有 “对话分组 → 约束调度” 的路径上将约束从单组扩展为多组集合。

## API设计
- **管理后台新增：** `POST /admin/channel-groups/chat-groups`（保存对话分组集合；空选择视为关闭）
- **管理后台保留：**
  - `POST /admin/channel-groups/{group_id}/set-chat`：将该分组加入对话分组集合
  - `POST /admin/channel-groups/{group_id}/unset-chat`：将该分组从对话分组集合移除

## 数据模型
无变更；仅扩展 `app_settings.chat_group_name` 的含义为“CSV 分组名列表”。

## 安全与性能
- **安全:** 不引入新敏感数据；不记录/回显明文凭据；分组名严格校验。
- **性能:** 每次解析对话分组仅做一次 `channel_groups WHERE name IN (...)` 查询（最多 20 个）。

## 测试与部署
- **测试:** `go test ./...`；补齐 openai handler 对多分组约束的单测。
- **部署:** 无迁移；发布后即可在管理后台勾选多分组生效。

