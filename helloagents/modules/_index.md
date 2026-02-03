# 模块索引

- `ci_github_actions.md`：CI（GitHub Actions）与 Codex CLI E2E 测试约定
- `admin_users.md`：管理后台用户管理（`/admin/users`），支持手动加余额（`POST /admin/users/{user_id}/balance`）
- `admin_channel_detail.md`：管理后台渠道详情（`/admin/channels/{channel_id}/detail`），按 Key/账号聚合展示近端用量与余额
- `upstream_request_policy.md`：上游渠道请求字段策略（`service_tier` / `store` / `safety_identifier`）
- `upstream_param_override.md`：上游渠道参数改写（`param_override`，new-api `operations` 兼容）
- `upstream_header_override.md`：上游渠道请求头覆盖（`header_override`，支持 `{api_key}` 变量替换）
- `upstream_status_code_mapping.md`：上游渠道状态码映射（`status_code_mapping`，仅改写对外 HTTP status）
- `upstream_body_filters.md`：上游渠道请求体黑白名单（`request_body_whitelist` / `request_body_blacklist`）
- `upstream_model_suffix_preserve.md`：上游渠道模型后缀保护名单（`model_suffix_preserve`，用于 Responses 推理后缀解析）
- `upstream_newapi_settings.md`：上游渠道 new-api 对齐设置项（`openai_organization/test_model/tag/remark/weight/auto_ban/setting`）
- `packaging_installers.md`：发布产物与安装包（Debian/Ubuntu `.deb`、Windows `realms.exe`）
- `web_spa.md`：前端 SPA（Vite + React）结构与 UI 风格基线（对齐 tag `0.3.3`）
