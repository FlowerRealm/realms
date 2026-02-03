# 变更提案: 站点地址（Site Base URL）统一

## 需求背景

当前系统在“对外链接/回调地址”生成上存在多种来源：

- 配置文件中的 `server.public_base_url`
- 运行时按请求推断的 `scheme://host`（含 `X-Forwarded-*` 信任逻辑）
- 少量场景（如回调后的回跳）依赖启动时注入的 base URL

当服务部署在反向代理/TLS 终止之后，或存在多入口访问时，如果站点对外地址没有被统一管理，就容易出现：

- 页面展示的 API 基础地址不正确（用户复制粘贴后不可用）
- 支付回调/返回地址不正确（第三方平台配置困难）
- OAuth 回调完成后的回跳地址不正确（体验差、难排查）

因此需要引入“站点地址”的明确概念，并让回调与前端展示统一使用该地址。

## 变更内容

1. 新增应用级设置项 `site_base_url`（持久化到 `app_settings`），作为“站点地址”的唯一可配置入口之一。
2. 站点地址解析优先级统一为：
   - `app_settings.site_base_url`（若存在且合法）
   - `server.public_base_url`（配置文件/环境变量，若存在且合法）
   - 否则按请求推断 `scheme://host`（信任代理头时按现有逻辑处理）
3. 将所有需要生成对外链接/回跳地址的逻辑统一切换到该“站点地址”。

## 影响范围

- **模块:** `internal/store`, `internal/config`, `internal/web`, `internal/admin`, `internal/codexoauth`
- **文件:**
  - `internal/store/app_settings.go`
  - `internal/config/config.go`
  - `internal/web/server.go`
  - `internal/admin/server.go`
  - `internal/admin/templates/settings.html`
  - `internal/codexoauth/flow.go`

## 核心场景

### 需求: 站点地址统一
**模块:** Web 控制台 / 管理后台 / 回调链路

#### 场景: 反向代理/TLS 终止部署
当服务通过反向代理对外提供 HTTPS 访问时：
- 页面展示的 API Base URL 应为 `https://<domain>/v1`（而不是内网/监听地址）
- 支付回调/返回地址应与对外域名一致
- OAuth 回调完成后应回跳到对外可访问的管理后台页面

## 风险评估

- **风险:** 配置了不合法的站点地址会导致页面展示与回调生成错误。
- **缓解:** 保存时做严格校验（仅允许 http/https 且 host 非空），运行时解析失败自动回退到配置文件/请求推断。

