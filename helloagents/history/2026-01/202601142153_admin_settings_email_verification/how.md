# 技术设计: 管理后台可配置邮箱验证码开关

## 技术方案

### 核心技术
- **存储:** MySQL 新表 `app_settings`（key/value + 时间戳）
- **开关读取优先级:** DB 覆盖值 > 配置文件默认值
- **UI:** SSR 管理后台增加 `/admin/settings`（root + CSRF）

### 实现要点
1. `app_settings` 表
   - key：`email_verification_enable`
   - value：`true/false`
2. store 封装
   - `GetBoolAppSetting/UpsertBoolAppSetting/DeleteAppSetting`
3. web 行为
   - 注册页/发送验证码接口/注册提交校验：统一读取“有效开关值”
4. admin UI
   - `GET /admin/settings`：展示开关与来源（UI 覆盖 / 配置默认）
   - `POST /admin/settings`：保存/恢复默认

## 测试与部署
- `go test ./...`
- DB 迁移会在启动时自动应用（`store.ApplyMigrations`）

