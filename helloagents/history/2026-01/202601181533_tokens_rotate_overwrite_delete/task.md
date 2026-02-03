# 任务清单: Token 重新生成覆盖 + 删除功能（轻量迭代）

目录: `helloagents/plan/202601181533_tokens_rotate_overwrite_delete/`

---

## 1. 重新生成覆盖（不再新增记录）
- [√] 1.1 为 `user_tokens` 增加“覆盖式轮换”存储方法：更新 `token_hash/token_hint`，并清理 `revoked_at`（必要时重置 `last_used_at`）
- [√] 1.2 更新 Web 控制台 `/tokens/rotate`：调用覆盖式轮换而不是 `CreateUserToken + RevokeUserToken`
- [√] 1.3 验证：重新生成后列表仍为同一条 Token 记录（ID 不变、Hint 更新、旧 key 立即失效）

## 2. Token 删除（硬删除）
- [√] 2.1 增加 `user_tokens` 删除存储方法（按 `user_id + token_id` 删除）
- [√] 2.2 增加 Web 控制台路由 `POST /tokens/delete`（Cookie 会话 + CSRF）
- [√] 2.3 更新 `/tokens` 页面：为已撤销 Token 提供“删除”入口（避免列表长期堆积）

## 3. 文档与验证
- [√] 3.1 更新 `helloagents/wiki/api.md`：修正 `/tokens/rotate` 行为描述，并补充 `/tokens/delete`
- [√] 3.2 更新 `helloagents/wiki/data.md`：补充 `user_tokens` 支持删除的说明（如需要）
- [√] 3.3 运行 `go test ./...`，确保编译与基础测试通过
