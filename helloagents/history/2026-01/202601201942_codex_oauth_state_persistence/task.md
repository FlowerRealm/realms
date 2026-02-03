# 任务清单: Codex OAuth state 持久化（修复手动粘贴回调易失效）

目录: `helloagents/plan/202601201942_codex_oauth_state_persistence/`

---

## 1. 持久化 pending state
- [√] 1.1 新增迁移：创建 `codex_oauth_pending` 表，用于保存 `state/endpoint_id/actor_user_id/code_verifier/created_at`
- [√] 1.2 在 `internal/store` 增加读写接口（create/get/delete/prune）

## 2. Flow 改造
- [√] 2.1 在 `internal/codexoauth/flow.go` 中：Start 时写入 DB；Complete/Callback 时从 DB 读取（避免进程重启/多实例导致 state 丢失）
- [√] 2.2 保留 `st=nil` 的内存兜底（用于单测），并更新相关测试

## 3. 文档同步
- [√] 3.1 更新知识库与变更记录：说明 state 在 DB 中缓存、有效期与常见失败原因

## 4. 测试
- [√] 4.1 运行 `go test ./...` 验证通过
