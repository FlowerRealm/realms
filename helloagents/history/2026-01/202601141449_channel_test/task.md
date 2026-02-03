# 任务清单: Channel 测试延迟与可用性指标

目录: `helloagents/plan/202601141449_channel_test/`

---

## 1. 数据与存储层（upstream_channels 扩展）
- [√] 1.1 在 `internal/store/migrations/0003_channel_test_fields.sql` 增加 `upstream_channels.last_test_*` 字段，验证 why.md#需求-channel-测试与结果展示-场景-root-在-channels-页面点击测试
- [√] 1.2 在 `internal/store/models.go` 扩展 `UpstreamChannel` 结构体并保持模板可用（time 可显示、int/bool 不显示指针），依赖任务 1.1
- [√] 1.3 在 `internal/store/upstreams.go` 更新 `ListUpstreamChannelsByGroup/GetUpstreamChannelByID/CreateUpstreamChannel` 的读写字段，并新增 `UpdateUpstreamChannelTest(...)`，依赖任务 1.2

## 2. 管理后台（入口 + 展示）
- [√] 2.1 在 `internal/server/app.go` 注册 `POST /admin/channels/{channel_id}/test` 路由（沿用 adminChain），验证 why.md#需求-channel-测试与结果展示-场景-root-在-channels-页面点击测试
- [√] 2.2 在 `internal/admin/server.go` 增加 `TestChannel` handler：校验权限→选择 endpoint/credential→调用 Executor→计时→回写 `last_test_*`，依赖任务 1.3、2.1
- [√] 2.3 在 `internal/admin/templates/channels.html` 展示 `last_test_*` 并增加“测试”按钮（POST+CSRF），依赖任务 2.2

## 3. 依赖注入与复用
- [√] 3.1 调整 `admin.NewServer(...)` 与 `internal/server/app.go` 的构造参数，把 `scheduler`/`upstream.Executor`（或等价接口）注入 admin Server，避免在 admin 层新建不一致的 HTTP client，依赖任务 2.2

## 4. 安全检查
- [√] 4.1 执行安全检查（按 G9：权限控制、SSRF 校验复用、响应体读取上限、错误信息不泄露凭据）

## 5. 文档更新（知识库）
- [√] 5.1 更新 `helloagents/wiki/data.md`：补充 `upstream_channels.last_test_*` 字段说明
- [√] 5.2 更新 `helloagents/wiki/modules/codex.md`：记录本次变更条目（变更历史）
- [√] 5.3 更新 `helloagents/CHANGELOG.md`：新增一条 Unreleased 记录（admin channel test 指标）

## 6. 测试
- [√] 6.1 在 `internal/admin/server_test.go`（或等价位置）补充测试：无可用 credential 时的错误路径、可用时能写入 `last_test_*`
- [√] 6.2 执行 `go test ./...` 验证无回归
  > 备注: 过程中补齐修复了仓库残留的旧配额相关代码导致的编译错误（`internal/quota/*`、`internal/codexoauth/jwt.go`），以保证本次改动可正常构建与测试。
