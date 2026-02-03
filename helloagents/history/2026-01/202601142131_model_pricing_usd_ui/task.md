# 任务清单: 模型定价单位改为 USD（图标展示）

目录: `helloagents/plan/202601142131_model_pricing_usd_ui/`

---

## 1. 管理后台表单（模型定价）
- [√] 1.1 更新 `internal/admin/templates/models.html`：定价输入改为 USD（支持小数），并用美元图标作为单位前缀展示
- [√] 1.2 更新 `internal/admin/models.go`：表单输入从 USD 转换为 `usd_micros` 存储（兼容旧的 micros 字段名）

## 2. 文档同步
- [√] 2.1 更新 `helloagents/wiki/data.md`：明确模型定价单位为 USD / 1M tokens（底层以 `usd_micros` 存储）
- [√] 2.2 更新 `helloagents/CHANGELOG.md`：记录单位与 UI 变更

## 3. 测试
- [√] 3.1 运行 `go test ./...`
