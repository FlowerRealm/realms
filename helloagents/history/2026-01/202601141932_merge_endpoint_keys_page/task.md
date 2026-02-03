# 任务清单: 合并 Endpoint 页面与 Key 管理页面

目录: `helloagents/plan/202601141932_merge_endpoint_keys_page/`

---

## 1. 管理后台 UI 合并

- [√] 1.1 在 `internal/admin/server.go` 中扩展 `Endpoints` handler：当 `Channel.Type == openai_compatible` 时加载 credentials 并渲染到 endpoints 页面，验证 why.md#需求-merge-endpoint-and-keys-场景-manage-keys-in-endpoint-page
- [√] 1.2 在 `internal/admin/templates/endpoints.html` 中内嵌 Keys 列表与“添加 Key” modal（复用现有字段 `name/api_key` 与删除表单），验证 why.md#需求-merge-endpoint-and-keys-场景-manage-keys-in-endpoint-page，依赖任务1.1
- [√] 1.3 删除旧 `GET /admin/endpoints/{endpoint_id}/openai-credentials` 页面（移除 handler/模板/路由），验证 why.md#需求-merge-endpoint-and-keys-场景-manage-keys-in-endpoint-page，依赖任务1.2
- [√] 1.4 在 `internal/admin/server.go` 中调整 `CreateOpenAICredential` / `DeleteOpenAICredential` 成功后的跳转目标到合并页（`/admin/channels/{channel_id}/endpoints`），验证 why.md#需求-merge-endpoint-and-keys-场景-manage-keys-in-endpoint-page，依赖任务1.3
- [√] 1.5 在 `internal/admin/templates/channels.html` 中更新 “管理密钥” 入口指向合并页（建议附带 `#keys` anchor），验证 why.md#需求-merge-endpoint-and-keys-场景-manage-keys-in-endpoint-page，依赖任务1.4

## 2. 安全检查

- [√] 2.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

## 3. 文档更新

- [√] 3.1 更新 `helloagents/CHANGELOG.md` 记录管理后台页面合并变更

## 4. 测试

- [√] 4.1 运行 `go test ./...`
- [?] 4.2 手动验证：访问 `/admin/channels/{id}/endpoints` 添加/删除 Key；确认旧 `GET /admin/endpoints/{endpoint_id}/openai-credentials` 书签/入口已移除（预期 404）
  > 备注: 当前环境未启动服务与浏览器，需你本地验证页面交互与路由行为。
