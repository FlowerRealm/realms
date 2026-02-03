# 技术设计: 站点地址（Site Base URL）统一

## 技术方案

### 核心技术
- Go（标准库 `net/url`）
- 既有 `app_settings` 持久化机制

### 实现要点

1. **新增统一的站点地址校验/归一化函数**
   - 在 `internal/config` 提供 `NormalizePublicBaseURL(raw string) (string, error)`：
     - 去空格、去尾部 `/`
     - 允许空字符串（表示未配置）
     - 仅允许 `http/https` 且 `host` 非空
2. **新增 `app_settings.site_base_url`**
   - 在 `internal/store/app_settings.go` 增加常量 `SettingSiteBaseURL`
   - 管理后台 `系统设置` 增加输入框，可保存/恢复默认
3. **统一站点地址解析优先级**
   - Web 控制台与管理后台的 `baseURLFromRequest` 逻辑前置读取 `app_settings.site_base_url`：
     - 命中且合法 → 直接返回
     - 否则沿用现有逻辑：配置文件 `server.public_base_url` → 请求推断
4. **回调回跳地址统一**
   - `internal/codexoauth.Flow` 在生成回跳 URL 时优先读取 `app_settings.site_base_url`，保证与页面展示一致。

## 安全与性能

- **安全:** 站点地址只用于拼接站内路径，不进行外部请求；仍对输入做协议与 host 校验，避免明显误配。
- **性能:** `site_base_url` 读取为一次简单的 `app_settings` 查询；页面与回调路径使用频率可接受。

## 测试与部署

- **测试:**
  - 增加 `NormalizePublicBaseURL` 的单元测试覆盖常见输入与错误输入
  - 运行 `go test ./...`
- **部署:**
  - 可继续使用 `server.public_base_url` 配置（兼容旧行为）
  - 如希望运行期调整对外地址，可在管理后台 `系统设置` 中配置 `站点地址`

