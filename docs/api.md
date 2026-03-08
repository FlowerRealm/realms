# API 使用说明

## 认证

### Web / 管理面

- 浏览器通过会话 Cookie 访问管理后台
- 可选使用 `REALMS_ADMIN_API_KEY` 调用 `/api/admin/*` 与 `/api/channel*`
- 管理员 Key 只用于管理面，不用于 `/v1/*`

### 数据面 `/v1/*`

- 使用用户 Token（在 `/tokens` 创建）
- 支持 `Authorization: Bearer <token>` 或 `x-api-key`

## 已移除能力

以下 personal 专属接口已删除：

- `POST /api/personal/bootstrap`
- `GET /api/personal/keys`
- `POST /api/personal/keys`
- `POST /api/personal/keys/{key_id}/revoke`
- `/api/admin/mcp*`
- `/api/admin/skills*`
- `/api/admin/personal-config*`
