# 任务清单: 管理后台可配置 Chat 联网搜索

目录: `helloagents/plan/202601181321_admin_search_settings/`

---

## 1. app_settings keys
- [√] 1.1 在 `internal/store/app_settings.go` 中新增联网搜索配置 key 常量，验证 why.md#需求-管理后台配置联网搜索-场景-启用-searxng-搜索

## 2. 管理后台 UI 与保存逻辑
- [√] 2.1 在 `internal/admin/templates/settings.html` 增加 SearXNG 配置表单与提示，验证 why.md#需求-管理后台配置联网搜索-场景-启用-searxng-搜索
- [√] 2.2 在 `internal/admin/server.go` 的 Settings/UpdateSettings 中读取与保存上述配置（含 reset 删除），验证 why.md#需求-管理后台配置联网搜索-场景-配置错误时提示

## 3. Web Chat 使用生效配置
- [√] 3.1 在 `internal/web/server.go` 中实现“搜索配置生效逻辑”（默认 + app_settings 覆盖），并用于 `/chat` 渲染
- [√] 3.2 在 `internal/web/chat.go` 的 `APIChatSearch` 中按生效配置执行搜索（不依赖启动时固定配置）

## 4. 文档更新
- [√] 4.1 更新 `helloagents/wiki/api.md`：补充“可在 /admin/settings 配置 search.searxng.*”
- [√] 4.2 更新 `helloagents/CHANGELOG.md`：记录新增管理后台配置能力

## 5. 测试
- [√] 5.1 运行 `go test ./...`
