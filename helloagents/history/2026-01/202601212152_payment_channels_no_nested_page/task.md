# task

> 轻量迭代：支付渠道管理扁平化（避免“配置详情”再跳一层页面）

- [√] 将支付渠道列表页的“配置详情”改为弹窗编辑
- [√] `GET /admin/settings/payment-channels/{payment_channel_id}` 重定向到列表页并自动打开编辑弹窗
- [√] `GET /admin/payment-channels/{payment_channel_id}` 兼容跳转到列表页并自动打开编辑弹窗
- [√] 补齐路由测试覆盖 `/admin/settings/payment-channels*`
- [√] 运行 `go test ./...`

