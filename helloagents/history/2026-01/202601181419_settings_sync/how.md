# 技术设计: 配置文件与管理后台系统设置项同步（最小方案）

## 技术方案

### 核心目标（按“最小同步”定义）

1. `/admin/settings` 的“可编辑运行期设置”在 `config.example.yaml` 中都有默认值/配置入口（允许通过“映射关系”而非完全同名）。
2. `config.example.yaml` 的“启动期配置项”在 `/admin/settings` 中以“只读清单”可见（不要求可编辑）。
3. 通过自动化校验阻止未来再次漂移。

### 核心技术

- Go 配置加载：`internal/config`（YAML + 默认值 + env overrides）
- 运行期设置：MySQL `app_settings`（`internal/store/app_settings.go`）
- 管理后台：模板渲染 `internal/admin/templates/settings.html` + handler `internal/admin/server.go`

### 实现要点

#### 1) 配置侧：补齐缺失键位（并可作为默认回退）

对缺失的 16 个键，新增一段“仅用于运行期设置缺省值”的配置结构（建议独立 section，避免与现有 `server/billing/payment/search/smtp` 结构混淆）：

- `app_settings_defaults.site_base_url`
- `app_settings_defaults.admin_time_zone`
- `app_settings_defaults.chat_group_name`
- `app_settings_defaults.feature_disable_*`（13 个）

约束：
- **向后兼容**：旧配置文件不需要修改；新 section 缺省为空/false。
- **优先级不变**：只在 `app_settings` 对应键不存在时才读取该默认值。

落点（预期）：
- `internal/config/config.go`: 增加结构体与默认值（`defaultConfig()`）。
- `config.example.yaml`: 增加对应配置示例，并清晰标注优先级：
  - `app_settings`（DB）优先
  - 否则用 `app_settings_defaults`（config 默认）
  - 仍为空时回退现有缺省逻辑（如 `server.public_base_url` / `Asia/Shanghai` / `false`）

#### 2) 运行期读取：对缺失键增加“config 默认值回退”

关键点是把“config 默认值”引入到目前仅依赖 `app_settings` 的路径中：

- `site_base_url`
  - 现状：多数位置已“优先 app_settings，否则回退 `server.public_base_url`/推断”。
  - 改动：在 app_settings 不存在时，优先使用 `app_settings_defaults.site_base_url`（若配置），再回退现有逻辑。
- `admin_time_zone`
  - 改动：app_settings 不存在时，优先 `app_settings_defaults.admin_time_zone`，否则保持现有默认（如 `Asia/Shanghai`）。
- `feature_disable_*`
  - 改动：在 feature flags 汇总处（如 `internal/store/features.go`）引入 config 默认值；app_settings 仍覆盖。
- `chat_group_name`
  - 改动：在读取对话分组设置处（`internal/store/chat_group.go` 等）增加 config 默认值回退；并确保后台分组页仍可覆盖。

#### 3) 管理后台 UI：增加启动期配置只读区块

在 `/admin/settings` 追加一个“启动期配置（只读）”区块，目标是“信息对称”：

- 覆盖的 config-only 分类（以 key 清单为主）：
  - `env`
  - `db.*`
  - `server.*`（监听地址、超时、public_base_url 等）
  - `limits.*`
  - `security.*`
  - `tickets.*`
  - `codex_oauth.*`
  - `self_mode.*`
- 安全策略：
  - **敏感值不回显**：对 DSN、token、secret、key 等字段用 `******/已设置` 替代
  - 或者只展示“键名 + 说明”，不展示当前值（更保守）

落点（预期）：
- `internal/admin/server.go`: 在 settings 页面数据中提供只读清单（必要时携带脱敏后的 value）。
- `internal/admin/templates/settings.html`: 渲染只读区块（建议 `details/summary` 折叠，避免页面过长）。

#### 4) 自动化校验：阻止设置项再次漂移

建议增加一个轻量校验（脚本或 Go 测试）：

- 输入来源：
  - `internal/store/app_settings.go` 的 setting keys 常量
  - `config.example.yaml` 的扁平化 key 清单
  - `internal/admin/templates/settings.html` 的表单字段（`name="..."`）
- 校验规则（最小同步版）：
  - 断言 16 个缺失键在 `config.example.yaml` 中存在（作为“运行期设置默认值”入口）
  - 断言 `/admin/settings` 可编辑区块包含关键运行期设置字段（避免误删）
  - 断言新增的“启动期配置只读清单”存在（避免被回滚）

落点（示例）：
- `scripts/check_settings_sync.py`（可复用项目现有 Python 环境）
- 或 Go test（更贴近 repo 的主语言）

## 安全与性能

- 不新增外部依赖服务；仅增加配置/页面展示与默认回退逻辑。
- 所有输入仍沿用现有校验（URL/时区/数字范围等）；新增字段需补齐同等级校验。
- 只读区块严格脱敏，避免泄露凭证类信息。

## 测试与部署

- 测试：
  - `go test ./...`
  - 运行同步校验脚本/测试（作为 CI gate）
- 部署：
  - 更新部署侧 `config.yaml`（如需要设置新的默认值 section）
  - 重启服务生效（启动期配置无热加载）

