# 任务清单: Web 对话会话管理（本地）

目录: `helloagents/plan/202601171948_chat_sessions/`

---

## 1. 前端存储与迁移
- [√] 1.1 在 `internal/web/templates/chat.html` 定义新 `localStorage` 结构（`realms_chat_sessions_v1`）与读写封装，验证 why.md#需求-会话检索与兼容迁移--场景-旧数据迁移
- [√] 1.2 在 `internal/web/templates/chat.html` 将“当前 state”重构为“当前会话视图”，确保发送消息仅读写当前会话，验证 why.md#需求-会话列表管理--场景-切换会话，依赖任务1.1

## 2. 会话列表 UI 与操作
- [√] 2.1 在 `internal/web/templates/chat.html` 增加会话侧栏（搜索框 + 会话列表 + 新建按钮），并实现会话切换渲染，验证 why.md#需求-会话列表管理--场景-切换会话，依赖任务1.2
- [√] 2.2 在 `internal/web/templates/chat.html` 实现新建/重命名/删除（Bootstrap modal），并处理删除当前会话的回退逻辑，验证 why.md#需求-会话列表管理--场景-新建会话 与 why.md#需求-会话列表管理--场景-删除会话，依赖任务2.1
- [√] 2.3 在 `internal/web/templates/chat.html` 实现会话搜索（标题+内容匹配），验证 why.md#需求-会话检索与兼容迁移--场景-搜索会话，依赖任务2.1

## 3. 导出与分享
- [√] 3.1 在 `internal/web/templates/chat.html` 实现“导出会话(JSON 下载)”与“分享会话(Markdown 复制)”，验证 why.md#需求-导出与分享--场景-导出会话 与 why.md#需求-导出与分享--场景-分享会话，依赖任务2.2

## 4. 安全检查
- [√] 4.1 执行安全检查（按G9：不新增明文凭据存储；分享/导出为显式动作；错误信息不泄露 token）

## 5. 文档更新
- [√] 5.1 更新 `helloagents/wiki/api.md` 中 `/chat` 的说明（多会话本地存储、导出/分享能力），并在 `helloagents/CHANGELOG.md` 记录变更

## 6. 验证
- [√] 6.1 运行 `go test ./...`
- [?] 6.2 手工验证：迁移旧数据、新建/切换/重命名/删除、搜索、导出(JSON)、分享(Markdown)、发送消息仍可流式输出
  > 备注: 需要在浏览器内回归验证（本次仅做了服务端/模板渲染与接口调用的基本检查）。
