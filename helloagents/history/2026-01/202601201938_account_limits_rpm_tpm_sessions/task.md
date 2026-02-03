# 任务清单: 按账号限额（RPM/TPM/会话数）

目录: `helloagents/plan/202601201938_account_limits_rpm_tpm_sessions/`

---

## 1. 数据模型与迁移
- [√] 1.1 新增迁移：为 `openai_compatible_credentials` / `codex_oauth_accounts` 增加 `limit_sessions/limit_rpm/limit_tpm`
- [√] 1.2 新增迁移：将 `upstream_channels.limit_cc` 改名为 `limit_sessions`（兼容历史语义）
- [√] 1.3 新增迁移：将 channel 级 limits 作为默认值回填到账号（仅在账号未显式设置时回填）

## 2. Store 层读写
- [√] 2.1 更新 `internal/store/models.go`：为 credential/account 增加 limits 字段；为 channel 将 `LimitCC` 改名为 `LimitSessions`
- [√] 2.2 更新 `internal/store/upstreams.go`：查询/扫描/写入字段从 `limit_cc` 切换到 `limit_sessions`；并补齐 credential/account 的 limits 查询
- [√] 2.3 增加 store 更新接口：支持更新 credential/account 的 limits（root-only 管理面调用）

## 3. Scheduler：按账号 sessions/rpm/tpm 生效
- [√] 3.1 更新 `internal/scheduler/state.go`：实现 sessions 计数（绑定占用）与过期清理；实现 TPM 滑动窗口统计
- [√] 3.2 更新 `internal/scheduler/scheduler.go`：在 credential/account 选择时应用 `limit_sessions/limit_rpm/limit_tpm` 过滤；保持 failover 语义
- [√] 3.3 更新 `internal/scheduler/scheduler_test.go`：覆盖 sessions/rpm/tpm 超限跳过、binding 复用与过期释放

## 4. API 数据面：TPM 统计回流
- [√] 4.1 更新 `internal/api/openai/handler.go`：当拿到 usage（非流式/流式）时，将 total tokens 计入对应 `CredentialKey` 的 TPM 统计
- [√] 4.2 清理/下线旧的 `cc` 并发逻辑：移除 channel 级并发限额的强制生效点（避免概念混淆）

## 5. 管理后台：按账号编辑 limits
- [√] 5.1 更新 `internal/admin/templates/endpoints.html`：在 credentials/accounts 列表中展示并编辑 `sessions/rpm/tpm`
- [√] 5.2 增加管理面 handler：`POST /admin/openai-credentials/{id}/limits` 与 `POST /admin/codex-accounts/{id}/limits`
- [√] 5.3 文档同步：更新 `helloagents/wiki/data.md`、`helloagents/wiki/api.md`、`helloagents/wiki/modules/realms.md`

## 6. 安全检查
- [√] 6.1 执行安全检查：输入校验、root-only 权限校验、确保不落盘敏感请求内容

## 7. 测试
- [√] 7.1 运行 `go test ./...` 验证通过
