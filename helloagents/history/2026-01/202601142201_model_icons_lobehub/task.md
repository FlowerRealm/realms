# 任务清单: 模型图标库引入（参考 new-api）

目录: `helloagents/plan/202601142201_model_icons_lobehub/`

---

## 1. 模型图标库接入
- [√] 1.1 新增 `internal/icons/model_icons.go`：提供 `modelIconURL`（ownedBy 优先，modelID 回退；CDN: @lobehub/icons-static-svg）
- [√] 1.2 在 `internal/web/server.go` 与 `internal/admin/server.go` 注入模板函数 `modelIconURL`

## 2. 控制台/管理后台 UI
- [√] 2.1 更新 `internal/web/templates/models.html`：Model ID 列展示品牌图标
- [√] 2.2 更新 `internal/admin/templates/models.html`：Public ID 列展示品牌图标
- [√] 2.3 更新 `internal/admin/templates/channel_models.html`：Public ID 列展示品牌图标
- [√] 2.4 更新 `internal/web/templates/base.html` 与 `internal/admin/templates/base.html`：补充 `.rlm-model-icon` 样式

## 3. 文档更新
- [√] 3.1 更新 `helloagents/wiki/modules/realms.md`：记录模型图标库接入方式与映射规则
- [√] 3.2 更新 `helloagents/CHANGELOG.md`：补充 Unreleased 记录

## 4. 测试
- [√] 4.1 执行 `go test ./...`，确保编译与测试通过
