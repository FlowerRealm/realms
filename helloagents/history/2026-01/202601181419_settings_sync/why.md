# 变更提案: 配置文件与管理后台系统设置项同步（最小方案）

## 需求背景

当前项目存在两套“可配置项”入口：

1. 启动期配置：`config.yaml` / `config.example.yaml`（YAML；启动时加载）
2. 运行期配置：管理后台「系统设置」`/admin/settings`（写入 `app_settings` 表；仅 root）

问题在于两边的设置项清单会漂移：
- 新增/调整设置项时，容易只改一边，导致“明明配置了却没生效/找不到入口/恢复默认不一致”等运维问题。
- 部分运行期设置项（如 `site_base_url`、`admin_time_zone`、`feature_disable_*`、`chat_group_name`）在 `config.example.yaml` 中没有对应键位，导致“配置示例不能作为完整清单”。
- 部分启动期配置（如 `db.*`、`server.*`、`limits.*`、`security.*`、`tickets.*`、`codex_oauth.*`、`self_mode.*`）在 `/admin/settings` 中不可见，导致排障需要翻配置文件/查部署，信息不对称。

本提案选择最小同步方案：不引入“后台写回 config.yaml / 热加载”，只保证**设置项清单在两边都能找到**，并在代码层加一道校验防止未来再次漂移。

## 变更内容

1. 补齐 `config.example.yaml` 缺失的 `app_settings` 相关键（共 16 个）：
   - `site_base_url`
   - `admin_time_zone`
   - `feature_disable_*`（13 个开关）
   - `chat_group_name`
2. 管理后台 `/admin/settings` 增加「启动期配置（只读）」区块：
   - 展示 config-only 的关键设置项清单（必要时显示安全脱敏后的值）
   - 明确提示“修改需在 `config.yaml` 中完成，且需要重启服务”
3. 增加同步校验（测试或脚本）：
   - 自动比对 `config.example.yaml`、`app_settings` 常量与 `/admin/settings` 页面字段，防止漏改

## 影响范围

- **模块:**
  - 配置加载：`internal/config`
  - 管理后台：`internal/admin`
  - 运行期设置存储：`internal/store`（`app_settings` / feature flags / chat group）
  - 站点地址计算：`internal/web`、`internal/codexoauth`（读取 `site_base_url`）
- **文件（预期）：**
  - `config.example.yaml`
  - `internal/config/config.go`
  - `internal/admin/server.go`
  - `internal/admin/templates/settings.html`
  - `internal/store/features.go`、`internal/store/chat_group.go`（如需补充默认回退）
  - `helloagents/wiki/modules/realms.md`、`README.md`（文档同步）

## 核心场景

### 需求: 设置项清单同步
**模块:** 配置 / 管理后台

#### 场景: 新增一个系统设置键
- 预期结果：开发者在 `/admin/settings` 与 `config.example.yaml` 都能找到对应设置项（或明确标注为“仅启动期/仅运行期”）。
- 预期结果：CI/测试能捕获“只改了一边”的漂移。

#### 场景: 运维排障需要确认启动期配置
- 预期结果：在 `/admin/settings` 中能看到启动期配置的“清单与说明”（必要时含脱敏后的当前值），无需到处翻文件。

#### 场景: 恢复为配置文件默认
- 预期结果：点击「恢复为配置文件默认」后，运行期设置恢复到 config 默认/缺省（不残留覆盖项）。

## 风险评估

- **风险：敏感信息泄露（EHRB 信号）**
  - `db.dsn`、`smtp_token`、`payment_*_key/secret/webhook_secret`、`security.subscription_order_webhook_secret` 等可能包含密钥。
  - 缓解：只读区块对敏感值只显示“已设置/******”，或仅展示键名不展示值。
- **风险：向后兼容**
  - 缓解：配置字段仅新增、保持默认值；不改变现有优先级（`app_settings` 仍覆盖 config 默认）。
- **风险：误操作导致锁死管理后台**
  - 缓解：延续现有护栏：`/admin/settings` 不应被 `feature_disable_*` 禁用；只读区块不提供写入能力。

