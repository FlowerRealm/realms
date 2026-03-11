# 迁移说明

项目已移除 `personal` / `app` 形态，统一为单一服务端模式。

## 如果你之前使用 personal / realms-app

你需要迁移到统一模式：

- 删除 `REALMS_MODE` 配置
- 删除 `REALMS_SUB2API_BASE_URL`、`REALMS_SUB2API_GATEWAY_KEY`、`REALMS_SUB2API_TIMEOUT_MS`
- 如需启用 `/v1/responses/compact`，改用 `REALMS_COMPACT_GATEWAY_BASE_URL`、`REALMS_COMPACT_GATEWAY_KEY`、`REALMS_COMPACT_GATEWAY_TIMEOUT_MS`
- 改用账号登录管理后台
- 改用用户 Token 访问 `/v1/*`
- 不再使用 personal 管理 Key、personal API Key、MCP/Skills 管理页

历史数据库中的 personal 相关表与设置会被忽略，但不会阻止服务启动。
