# 任务清单: admin_settings_email_verification

目录: `helloagents/plan/202601142153_admin_settings_email_verification/`

---

## 1. 数据层
- [√] 1.1 新增迁移 `internal/store/migrations/0003_app_settings.sql` 创建 `app_settings`
- [√] 1.2 新增 `internal/store/app_settings.go` 实现读取/写入/删除布尔开关

## 2. 管理后台（UI）
- [√] 2.1 新增 `internal/admin/templates/settings.html` 并在侧边栏加入入口
- [√] 2.2 在 `internal/admin/server.go` 增加 `Settings/UpdateSettings` handler
- [√] 2.3 在 `internal/server/app.go` 注册 `/admin/settings` 路由

## 3. Web 行为对齐
- [√] 3.1 在 `internal/web/server.go` 统一读取 DB 覆盖值并作为注册/渲染依据

## 4. 文档
- [√] 4.1 更新 `helloagents/wiki/api.md` 增加 `/admin/settings`
- [√] 4.2 更新 `helloagents/wiki/data.md` 记录 `app_settings`
- [√] 4.3 更新 `helloagents/CHANGELOG.md` 记录新增能力

## 5. 测试
- [√] 5.1 运行 `go test ./...`
