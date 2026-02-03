# 任务清单: Realms 品牌改名（移除 Codex 作为产品名）

目录: `helloagents/history/2026-01/202601141611_rebrand_realms/`

---

## 1. 服务入口与构建产物
- [√] 1.1 将入口从 `cmd/codex` 迁移为 `cmd/realms`，并同步更新 `.air.toml` / `Dockerfile` 的构建输出与启动入口，验证 why.md#需求-品牌统一为-realms-场景-开发者启动服务
- [√] 1.2 更新 `scripts/dev.sh` 的环境变量生成逻辑为 `REALMS_MASTER_KEY_BASE64`，验证 why.md#需求-品牌统一为-realms-场景-开发者启动服务

## 2. 配置与环境变量
- [√] 2.1 在 `internal/config/config.go` 中将 `CODEX_*` 覆盖项改为 `REALMS_*`（不保留兼容别名），验证 why.md#需求-品牌统一为-realms-场景-开发者启动服务
- [√] 2.2 更新 `internal/server/app.go` 中 master key 缺失提示，确保指向 `REALMS_MASTER_KEY_BASE64`

## 3. Web 控制台（用户界面）
- [√] 3.1 替换 `internal/web/templates/*.html` 中的品牌文案为 Realms（保留 Codex OAuth 术语），验证 why.md#需求-品牌统一为-realms-场景-用户访问登录页与后台
- [√] 3.2 更新 `internal/web/server.go` 中页面标题与 `SessionCookieName`（改为 realms），验证 why.md#需求-品牌统一为-realms-场景-用户访问登录页与后台

## 4. 管理后台（Admin）
- [√] 4.1 替换 `internal/admin/templates/*.html` 中的品牌文案为 Realms（保留 Codex OAuth 术语），验证 why.md#需求-品牌统一为-realms-场景-用户访问登录页与后台
- [√] 4.2 更新 `internal/admin/server.go` 中页面标题为 Realms

## 5. 默认开发资源与文档
- [√] 5.1 更新 `docker-compose.yml` / `config.example.yaml` 中示例数据库名与说明为 `realms`
- [√] 5.2 更新 `README.md` 的启动命令、环境变量名与服务名称为 Realms

## 6. 安全检查
- [√] 6.1 执行安全检查（按G9: 不引入明文密钥、避免新增高风险操作、确认仅命名/文案变更不影响鉴权/权限）

## 7. 知识库同步
- [√] 7.1 更新 `helloagents/project.md` / `helloagents/wiki/overview.md` / `helloagents/wiki/modules/*` 的模块命名与入口链接（Realms），并补充本次变更索引

## 8. 测试
- [√] 8.1 执行 `go test ./...`，确保测试通过
