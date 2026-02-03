# 任务清单：用户 Token 支持查看/复制（Web 控制台）

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 需求决策（阻断）

- [√] 确认存储方案：`token_plain` 纯明文 vs 可逆加密（已选：纯明文）
- [√] 确认撤销 Token 是否允许 reveal（已选：不允许）
- [√] 确认用户 Token 前缀（已选：`sk_`）

## B. 数据库与 Store

- [√] 新增 MySQL 迁移：`user_tokens` 增加 `token_plain` 字段
- [√] 更新 SQLite 初始化 schema：`user_tokens` 增加 `token_plain`
- [√] 为既有 SQLite DB 增加 ensure：自动补齐 `token_plain` 列
- [√] `CreateUserToken` 写入 `token_plain`
- [√] `RotateUserToken` 更新 `token_plain`
- [√] 新增 Store 方法：按 `user_id + token_id` 读取 `token_plain`（撤销态不允许；旧 token 不可 reveal）

## C. 后端 API

- [√] 新增路由：`GET /api/token/:token_id/reveal`（返回 `{token_id, token}`，并 `Cache-Control: no-store`）
- [√] 旧令牌（`token_plain` 为空）给出明确提示：需重新生成
- [√] 创建/轮换 Token 使用 `sk_` 前缀

## D. Web 控制台

- [√] `web/src/api/tokens.ts`：新增 reveal API 调用与类型
- [√] `web/src/pages/TokensPage.tsx`：新增“查看/隐藏”按钮（默认隐藏）
- [√] `web/src/pages/TokensPage.tsx`：新增“复制”按钮（复制 token 本身；必要时先 reveal）
- [√] `web/src/pages/TokensPage.tsx`：更新页面文案与安全提示
- [√] `web/src/pages/TokensPage.tsx`：撤销态 token 的交互策略（撤销后不允许 reveal/copy）

## E. 测试与验证

- [√] Go：新增/更新路由测试覆盖 reveal（含：成功、旧令牌无明文、撤销态策略）
- [√] Go：`go test ./...`
- [√] Web：`npm --prefix web run lint`
- [√] Web：`npm --prefix web run build`
- [-]（可选）Playwright：新增 `/tokens` reveal/copy 的 E2E smoke

## F. 文档与知识库同步

- [√] `README.md` / `docs/api.md`：同步 Token 前缀与“查看/复制”行为说明
- [√] `helloagents/modules/web_spa.md`：补充 `/tokens` 页面交互约定（查看/复制）
- [√] `helloagents/CHANGELOG.md`：记录本次变更（含 ⚠️ EHRB：敏感数据 - 用户已确认风险）
