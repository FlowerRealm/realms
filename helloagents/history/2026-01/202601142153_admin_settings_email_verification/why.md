# 变更提案: 管理后台可配置邮箱验证码开关

## 需求背景

邮箱验证码功能已支持通过配置文件 `email_verification.enable` 控制，但这属于“部署期配置”。实际运维中，希望管理员能在 UI 中快速开启/关闭该能力（例如：临时关闭注册验证码、或上线后开启强制校验），避免每次修改配置并重启服务。

## 变更内容

1. 新增管理后台“系统设置”页面 `GET /admin/settings`，提供“启用注册邮箱验证码”开关。
2. 开关持久化到数据库（运行期配置），使变更立即生效且跨重启保存。
3. 保留配置文件默认值：当数据库无覆盖值时，以 `email_verification.enable` 为默认行为（KISS + 兼容）。

## 影响范围

- **模块:** `internal/store`、`internal/admin`、`internal/web`、`internal/server`
- **API:** 新增管理面设置入口（SSR）：`/admin/settings`（GET/POST）
- **数据:** 新增表 `app_settings`（少量运行期开关）

## 核心场景

### 需求: UI 开关邮箱验证码
**模块:** admin / store / web

#### 场景: 管理员开启验证码
root 在 `/admin/settings` 打开“启用注册邮箱验证码”，保存后：
- 注册页显示验证码输入框与“发送验证码”按钮
- `POST /api/email/verification/send` 对外开放
- 注册提交必须校验验证码

#### 场景: 管理员恢复为配置文件默认
root 在 `/admin/settings` 点击“恢复为配置文件默认”：
- 删除 DB 覆盖项
- 行为回退到 `config.yaml` 的 `email_verification.enable`

