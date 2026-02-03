# 任务清单: 计费金额改用小数（移除 micros/分）

目录: `helloagents/plan/202601180742_billing_decimal/`

---

## 1. 数据库迁移
- [√] 1.1 新增迁移 `internal/store/migrations/0031_billing_decimal.sql`：将金额字段改为小数并执行缩放转换
- [√] 1.2 新增迁移 `internal/store/migrations/0032_billing_app_settings_decimal.sql`：迁移 app_settings 的 billing 配置键与值

## 2. Store 数据层
- [√] 2.1 更新 `internal/store/models.go` 金额字段类型与命名
- [√] 2.2 更新 `internal/store/*` 中涉及金额字段的 SQL（余额/用量/订阅/订单/上游余额）

## 3. 配额与计费
- [√] 3.1 更新 `internal/quota/*` 的成本估算与倍率逻辑，统一使用小数金额

## 4. Web 与支付回调
- [√] 4.1 更新 `internal/web/server.go` 的金额解析与格式化、充值/订阅页面展示与下单逻辑
- [√] 4.2 更新 `internal/server/payment_webhooks.go` 回调金额校验逻辑（Stripe/EPay）

## 5. 管理后台与配置
- [√] 5.1 更新 `internal/config/config.go` 与 `config.example.yaml` 的 billing 配置字段（小数金额/比例）
- [√] 5.2 更新 `internal/admin/*` 的设置页与金额展示（包含渠道分组倍率）

## 6. 安全检查
- [√] 6.1 执行安全检查（金额输入校验、敏感信息处理、权限控制、避免 SQL 拼接）

## 7. 文档更新
- [√] 7.1 更新 `helloagents/wiki/data.md` 与 `helloagents/wiki/modules/realms.md`：同步字段命名与单位变化
- [√] 7.2 更新 `helloagents/wiki/api.md`：同步用量接口金额单位说明
- [√] 7.3 更新 `helloagents/CHANGELOG.md`：记录本次破坏性变更

## 8. 测试
- [√] 8.1 运行 `go test ./...` 并修正编译/单测问题
