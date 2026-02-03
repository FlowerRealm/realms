# 技术设计: 用户可见界面与 README 全中文化

## 技术方案

### 核心技术
- Go `html/template`（SSR 模板）
- 现有页面路由与渲染逻辑（不引入 i18n 框架，不增加运行时依赖）

### 实现要点

1. **只改“固定文案”，不动数据与标识符**
   - 固定文案：模板中的按钮/标题/列名/提示/页脚等静态文本。
   - 保持不变：产品名 `Realms`、代码标识符、API 路径与参数名、JSON 字段名、配置 key、环境变量名、命令行 flag、示例代码块。
2. **术语统一**
   - 以“Codex 中转站”语境为准，避免同一概念多种中文/英文混用。
3. **最小变更**
   - 不引入语言包/字典映射，不做 locale 切换；仅做文本替换与少量排版对齐。

## 术语与翻译规则

### 术语表（统一口径）

- Dashboard → 控制台 / 仪表盘（按页面语义选择其一，避免混用）
- Admin / Admin Panel → 管理后台 / 管理面板
- Signed in as → 当前登录
- Token → 令牌（参考 new-api 的常见译法）
- API Tokens → API 令牌
- Model ID → 模型 ID
- Owner → 归属方
- Pricing → 计费 / 价格（结合上下文；涉及单位时保留 `(/ 1M tokens)` 等技术表达）
- Status → 状态
- Available → 可用
- Input / Output / Cache（计费语境）→ 输入 / 输出 / 缓存
- Terminal（UI 标签）→ 终端
- UTC Time / UTC → UTC 时间 / UTC（保留 UTC 作为专有缩写）

### 保留清单（不翻译）

- 产品/项目名：`Realms`、`Codex`、`OpenAI`、`OAuth`
- 路径与协议：`/v1/...`、URL、`Authorization: Bearer <token>`、`x-api-key`
- 环境变量/配置项/命令：`OPENAI_BASE_URL`、`OPENAI_API_KEY`、`security.allow_open_registration`、`go run ...`、`docker compose ...`
- 代码块内容（避免用户复制失败），除非明确是展示用文案（本次默认不改代码块）

## 变更策略

1. **Web 用户控制台**
   - 目标目录：`internal/web/templates/`
   - 将残留英文（如导航中的 `API Tokens`、页脚 `Realms Proxy Service`、表格列名等）替换为中文。
   - 将 `internal/web/server.go` 中的页面 Title 文案统一中文（保留 `Realms`）。
2. **管理后台**
   - 目标目录：`internal/admin/templates/`
   - 将残留英文（如 `Admin`、`Signed in as`、`Admin Panel`、`UTC Time`、`App` 等）替换为中文。
   - 将 `internal/admin/server.go` 中的页面 Title 统一中文（保留 `Realms`）。
3. **README**
   - 将 README 中的英文说明词替换为中文，统一术语（如：channel/endpoint/credential → 渠道/端点/凭证）。

## 测试与验证

- 静态校验：对目标文件执行英文残留扫描（例如匹配 `[A-Za-z]{2,}`），确认仅剩“保留清单”中的专有名词/技术标识符。
- 运行校验：`go test ./...` 确认无编译/测试回归。
- 体验校验（人工）：启动服务后逐页浏览（登录/注册/控制台/令牌/模型/订阅/用量/管理后台各页面），确认文案一致、无布局破坏。

