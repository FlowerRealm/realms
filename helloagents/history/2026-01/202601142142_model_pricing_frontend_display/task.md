# 任务清单: 模型定价作为前端展示项

目录: `helloagents/plan/202601142142_model_pricing_frontend_display/`

---

## 1. Web 控制台模型列表展示定价（/models）
- [√] 1.1 更新 `internal/web/server.go`：模型视图结构增加 input/output/cache 三类价格（USD / 1M tokens）
- [√] 1.2 更新 `internal/web/templates/models.html`：新增“定价”列并使用美元图标展示单位

## 2. 管理后台模型列表展示定价（/admin/models）
- [√] 2.1 更新 `internal/admin/templates/models.html`：列表新增“定价”列，展示 input/output/cache 三类价格（USD / 1M tokens）

## 3. 文档同步
- [√] 3.1 更新 `helloagents/wiki/api.md`：补充 `/models` 页面展示定价说明
- [√] 3.2 更新 `helloagents/CHANGELOG.md`：记录前端展示变更

## 4. 测试
- [√] 4.1 运行 `go test ./...`
