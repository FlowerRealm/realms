# 任务清单: 配置文件与管理后台系统设置项同步（最小方案）

目录: `helloagents/plan/202601181419_settings_sync/`

---

## 1. 配置示例补齐（缺失键位）
- [√] 1.1 在 `internal/config/config.go` 增加 `app_settings_defaults` 结构体与默认值（至少覆盖 `admin_time_zone`、`feature_disable_*`），验证 why.md#需求-设置项清单同步-场景-新增一个系统设置键
- [√] 1.2 在 `config.example.yaml` 增加缺失的 16 个键位（`site_base_url`、`admin_time_zone`、`feature_disable_*`、`chat_group_name`）并写明优先级与用途，验证 why.md#需求-设置项清单同步-场景-新增一个系统设置键

## 2. 运行期默认回退（不改变 app_settings 优先级）
- [√] 2.1 在 `internal/admin/server.go` 中：当 `site_base_url`/`admin_time_zone` 未被 `app_settings` 覆盖时，使用 `app_settings_defaults` 作为缺省，验证 why.md#需求-设置项清单同步-场景-恢复为配置文件默认
- [√] 2.2 在 `internal/store/features.go` 中：对 `feature_disable_*` 增加 config 默认回退（`app_settings` 仍覆盖），验证 why.md#需求-设置项清单同步-场景-恢复为配置文件默认
- [√] 2.3 在 `internal/store/chat_group.go`（或对应读取处）中：对 `chat_group_name` 增加 config 默认回退（后台分组页仍可覆盖），验证 why.md#需求-设置项清单同步-场景-新增一个系统设置键
- [√] 2.4 在 `internal/web/server.go` 与 `internal/codexoauth/flow.go` 中：`site_base_url` 读取逻辑补充 config 默认回退（若当前代码已满足则仅加测试/注释），验证 why.md#需求-设置项清单同步-场景-运维排障需要确认启动期配置

## 3. 管理后台设置页补齐（只读启动期配置）
- [√] 3.1 在 `internal/admin/server.go` 增加“启动期配置只读清单”的数据组装（含必要的脱敏），验证 why.md#需求-设置项清单同步-场景-运维排障需要确认启动期配置
- [√] 3.2 在 `internal/admin/templates/settings.html` 增加“启动期配置（只读）”区块（建议折叠显示），并对敏感项仅显示 `******/已设置`，验证 why.md#需求-设置项清单同步-场景-运维排障需要确认启动期配置
- [√] 3.3 确认 `/admin/settings` 仍具备安全护栏（不会被 `feature_disable_*` 锁死），验证 why.md#风险评估

## 4. 同步校验与测试
- [√] 4.1 新增同步校验：`scripts/check_settings_sync.py`（或 Go test）解析 `config.example.yaml`、`internal/store/app_settings.go`、`internal/admin/templates/settings.html`，至少断言“16 个缺失键已补齐”，验证 why.md#需求-设置项清单同步-场景-新增一个系统设置键
- [√] 4.2 运行 `go test ./...` 并跑同步校验，记录结果

## 5. 文档更新
- [√] 5.1 更新 `helloagents/wiki/modules/realms.md`：补充 `app_settings` 与 config 默认值/优先级说明，并列出新增的 `app_settings_defaults` section，验证 why.md#需求-设置项清单同步
- [√] 5.2（可选）更新 `README.md`：补充“系统设置与配置文件优先级”与排障入口
