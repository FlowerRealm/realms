# 任务清单: 支付渠道移除优先级字段

目录: `helloagents/plan/202601172128_payment_channel_remove_priority/`

---

## 1. 代码改动
- [√] 1.1 移除 store 层 `PaymentChannel.Priority` 与相关 SQL（`internal/store/models.go`、`internal/store/payment_channels.go`）
- [√] 1.2 移除管理后台支付渠道的 priority 字段读写与展示（`internal/admin/payment_channels.go`、`internal/admin/templates/payment_channels.html`、`internal/admin/templates/payment_channel.html`）
- [√] 1.3 更新 `payment_channels` 迁移：不再创建 `priority` 列与相关索引（`internal/store/migrations/0028_payment_channels.sql`）

## 2. 文档更新
- [√] 2.1 更新数据模型文档：`helloagents/wiki/data.md`
- [√] 2.2 更新变更记录：`helloagents/CHANGELOG.md`

## 3. 测试
- [√] 3.1 运行 `go test ./...`
