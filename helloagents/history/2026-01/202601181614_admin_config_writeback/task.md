# 任务清单: 管理后台写回配置文件（UI 配置全部项）

目录: `helloagents/history/2026-01/202601181614_admin_config_writeback/`

---

## 1. 配置加载支持（无 env 覆盖校验）
- [√] 1.1 在 `internal/config/config.go` 新增 `LoadFromFile(path string)`（不应用 env overrides），用于保存前校验 why.md#核心场景-场景-管理员保存了错误配置

## 2. 新增管理后台配置文件页
- [√] 2.1 在 `internal/admin/server.go` 增加 `Config`/`UpdateConfig` handler：读取 config 文件、校验、原子写回、可选 SIGTERM 重启
- [√] 2.2 新增模板 `internal/admin/templates/config.html`：YAML 编辑器 + 保存/保存并重启按钮

## 3. 接入路由与导航
- [√] 3.1 在 `internal/server/app.go` 注册 `GET/POST /admin/config`（adminChain）
- [√] 3.2 在 `internal/admin/templates/base.html` 增加侧边栏入口「配置文件」
- [√] 3.3 在 `cmd/realms/main.go` 将 `-config` 路径传入应用层（用于写回目标文件）

## 4. 校验与文档
- [√] 4.1 运行 `go test ./...`
- [√] 4.2 更新 `helloagents/wiki/modules/realms.md`/`README.md`：说明该功能依赖 supervisor，且多实例需共享卷/分发

## 5. 收尾
- [√] 5.1 更新 `helloagents/CHANGELOG.md`
- [√] 5.2 更新 `helloagents/history/index.md`
- [√] 5.3 迁移方案包至 `helloagents/history/2026-01/202601181614_admin_config_writeback/`
