# 任务清单: Codex OAuth 用量展示口径修复（5 小时 / 每周）

目录: `helloagents/plan/202601171959_codex_oauth_quota_display/`

---

## 1. 管理后台展示（admin）
- [√] 1.1 在 `internal/admin/server.go` 中补全额度窗口展示信息（中文标签、剩余 USD、重置时间），验证 why.md#需求-管理后台可读的额度窗口展示-场景-查看账号额度正常
- [√] 1.2 在 `internal/admin/templates/endpoints.html` 中替换“主额度/次额度”文案，并在进度条下展示“剩余/重置”细节行，验证 why.md#需求-管理后台可读的额度窗口展示-场景-查看账号额度正常
- [√] 1.3 保持错误降级：`QuotaError` 存在时只展示错误信息，验证 why.md#需求-管理后台可读的额度窗口展示-场景-上游拉取失败错误

## 2. 安全检查
- [√] 2.1 执行安全检查（按G9：敏感信息不落日志/不出页面、权限控制不变、无EHRB风险）

## 3. 文档更新
- [√] 3.1 更新 `helloagents/wiki/modules/realms.md` 记录 primary/secondary window 的口径映射与金额计算规则
- [√] 3.2 更新 `helloagents/CHANGELOG.md` 记录本次“管理后台额度展示修复”

## 4. 测试
- [√] 4.1 在 `internal/admin/*_test.go` 增加单元测试：剩余金额计算与展示格式化（0/50/100/超范围、resetAt nil），验证点：输出稳定、无 panic、边界正确
