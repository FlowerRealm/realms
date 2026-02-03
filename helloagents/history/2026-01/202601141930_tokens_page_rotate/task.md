# 任务清单: Web 控制台 Token 管理分页 + 一键重新生成

目录: `helloagents/history/2026-01/202601141930_tokens_page_rotate/`

---

## 1. Tokens 独立页面
- [√] 1.1 左侧导航新增 `API Tokens` 入口（`GET /tokens`）
- [√] 1.2 将 Token 管理从 `/dashboard` 迁移到 `/tokens`

## 2. Token 重新生成（rotate）
- [√] 2.1 增加 `POST /tokens/rotate`：生成新 Token 并撤销旧 Token
- [√] 2.2 Token 生成页支持一键复制与返回 Token 列表

## 3. 文档与记录
- [√] 3.1 更新 `README.md`：说明 Token 可随时重新生成
- [√] 3.2 更新 `helloagents/wiki/api.md`：补齐 `/tokens` 与 `/tokens/rotate`
- [√] 3.3 更新 `helloagents/CHANGELOG.md`：记录本次变更

## 4. 测试
- [√] 4.1 执行 `go test ./...`
