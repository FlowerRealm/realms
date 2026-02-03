# 技术设计: Web 对话会话管理（本地）

## 技术方案

### 核心技术
- Web SSR 模板：`internal/web/templates/chat.html`
- 前端：原生 JavaScript（不引入新依赖）
- UI：Bootstrap 组件（侧栏布局 + modal）
- 存储：浏览器 `localStorage`

### 实现要点
- **数据结构：** 将原先单一 `state={model,messages}` 升级为：
  - `sessions`: 会话数组（或 id->session 映射）
  - `current_session_id`: 当前会话 id
  - 每个 session：`id/title/model/messages/created_at/updated_at`
- **兼容迁移：** 若检测到旧 key `realms_chat_state_v1` 且新 key 不存在：
  - 将旧数据包装为单个会话写入新结构并设为当前会话
  - 迁移完成后移除旧 key（避免重复/歧义）
- **会话切换：** 切换仅影响 UI 与当前会话指针；发送消息时只读取/写入当前会话。
- **重命名/删除：** 使用 Bootstrap modal 进行输入与确认，避免 `prompt/confirm`（一致性与可控性更好）。
- **搜索：** 前端对会话列表做过滤，匹配范围：标题 + message 内容（大小写不敏感）。
- **导出：** `Blob + URL.createObjectURL + <a download>` 下载 JSON（当前会话）。
- **分享：** 生成 Markdown 文本并调用 `navigator.clipboard.writeText`；失败则在 modal/textarea 中提供可手动复制文本。

## API 设计
无新增/变更：
- 继续由浏览器直连 `POST /v1/responses`（Authorization 使用对话页专用 token）
- 模型列表 `GET /api/chat/models`
- token 轮换 `POST /api/chat/token`

## 数据模型（localStorage）

### Key 约定
- 旧（兼容读取/迁移）：`realms_chat_state_v1`
- 新：
  - `realms_chat_sessions_v1`：整体会话数据（JSON）
  - `realms_chat_token_v1`：对话 token（沿用）

### `realms_chat_sessions_v1` 结构（示例）
```json
{
  "version": 1,
  "current_session_id": "s_...",
  "sessions": [
    {
      "id": "s_...",
      "title": "历史会话",
      "model": "gpt-4.1-mini",
      "messages": [
        {"role": "user", "content": "你好"},
        {"role": "assistant", "content": "你好！"}
      ],
      "created_at": 1700000000000,
      "updated_at": 1700000000000
    }
  ]
}
```

## 安全与性能
- **安全:**
  - 不将会话内容上送服务端；分享/导出均为用户显式动作。
  - UI 增加“分享/导出可能包含敏感信息”的提示文案。
- **性能:**
  - `localStorage` 读写集中在节流保存（沿用现有 `saveStateSoon` 思路，扩展为保存 sessions）。
  - 搜索在输入时做轻量过滤；会话量较大时可增加简单的 debounce（先不引入复杂索引）。

## 测试与部署
- **测试:**
  - `go test ./...`（确保模板与服务端编译通过）
  - 手工回归：新建/切换/重命名/删除、搜索、导出、分享、旧数据迁移
- **部署:**
  - 无数据库迁移、无新 API；发布后用户首次打开 `/chat` 自动完成本地迁移

