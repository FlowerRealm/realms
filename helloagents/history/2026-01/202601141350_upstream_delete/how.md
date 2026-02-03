# 技术设计: 管理后台上游硬删除

## 技术方案

### 核心技术
- Go `net/http`（SSR 管理后台）
- MySQL（硬删除：物理删除 + 事务级联）

### 实现要点
- **Store 层新增硬删除方法**
  - `DeleteUpstreamChannel`（级联删除 endpoints/credentials/accounts）
  - `DeleteUpstreamEndpoint`（级联删除 credentials/accounts）
  - `DeleteOpenAICompatibleCredential`
  - `DeleteCodexOAuthAccount`
- **权限与归属校验**
  - `root` 可跨组操作
  - `group_admin` 仅允许操作本组资源（通过 endpoint→channel 反查 group_id 校验）
- **UI 与交互**
  - 列表页增加 status 展示
  - 展示“彻底删除”按钮（带 confirm），且所有行都允许删除（包括历史遗留的 status=0 行）

## API 设计

均为管理后台 SSR 表单提交（Cookie 会话 + CSRF）：

- `POST /admin/channels/{channel_id}/delete`
- `POST /admin/endpoints/{endpoint_id}/delete`
- `POST /admin/openai-credentials/{credential_id}/delete`
- `POST /admin/codex-accounts/{account_id}/delete`

## 安全与性能
- **安全:** 受 `RequireRoles(root|group_admin)` 与 CSRF 保护；删除前校验资源归属。
- **性能:** 删除 endpoint/channel 时为多条 DELETE（事务内）；列表查询不变。

## 测试与部署
- `go test ./...`
- 通过管理后台手工创建资源并删除，观察调度是否停止选择该资源。
