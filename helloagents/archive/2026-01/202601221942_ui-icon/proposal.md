# 变更提案: ui-icon

## 元信息
```yaml
类型: 优化
方案类型: implementation
优先级: P2
状态: 已完成
创建: 2026-01-22
```

---

## 1. 需求

### 背景
项目根目录临时放置了 `realms_icon.svg`，目前 Web 控制台与管理后台仍在使用占位图标（Material/Remix icon）。

需要将该图标放到项目中可维护的资源位置，并在 UI 中全站替换使用（favicon + 顶部/侧边栏 Logo + 登录/注册页 Header）。

### 目标
- 将 `realms_icon.svg` 迁移到稳定的资源目录，避免继续堆放在仓库根目录
- 提供统一可引用的图标 URL：`/assets/realms_icon.svg`
- 在 Web 控制台与管理后台全站替换品牌图标入口（ABC）
- 兼容浏览器自动请求的 `/favicon.ico`

### 约束条件
```yaml
时间约束: 无
性能约束: 图标请求应可被缓存
兼容性约束: favicon 需兼容现代浏览器；/favicon.ico 需兜底
业务约束: 无
```

### 验收标准
- [x] `realms_icon.svg` 不再位于仓库根目录，迁移到 `internal/assets/realms_icon.svg`
- [x] `GET /assets/realms_icon.svg` 返回 `200` 且 `Content-Type` 为 `image/svg+xml`
- [x] Web 控制台与管理后台 `<head>` 中设置 favicon，且全站使用新图标替换原占位图标
- [x] `GET /favicon.ico` 永久重定向到 `/assets/realms_icon.svg`
- [x] `go test ./...` 通过

---

## 2. 方案

### 技术方案
- 新增 `internal/assets` 包，将 `realms_icon.svg` 通过 `go:embed` 嵌入到二进制中，避免依赖运行时文件路径。
- 在 `internal/server/app.go` 注册静态资源路由：
  - `GET/HEAD /assets/realms_icon.svg`：直接返回 SVG（带缓存头）
  - `GET/HEAD /favicon.ico`：永久重定向到 `/assets/realms_icon.svg`
- 更新模板：
  - `internal/web/templates/base.html`：favicon + 侧边栏品牌图标 + 登录/注册页 Header 图标
  - `internal/admin/templates/base.html`：favicon + 侧边栏品牌图标

### 影响范围
```yaml
涉及模块:
  - internal/assets: 新增嵌入式静态资源
  - internal/server: 新增静态资源路由与测试
  - internal/web: base 模板替换品牌图标与 favicon
  - internal/admin: base 模板替换品牌图标与 favicon
预计变更文件: 6
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| SVG favicon 在部分环境不生效 | 中 | 提供 `/favicon.ico` 永久重定向兜底；同时在模板显式声明 `rel="icon"` + `type="image/svg+xml"` |
| 缓存导致图标更新不即时 | 低 | 设置 `Cache-Control: public, max-age=86400`；如后续需要可加版本化路径 |

---

## 3. 技术设计（可选）

本变更不涉及业务 API 设计与数据模型变更；仅增加静态资源路由与模板引用。

### 架构设计
（无）

### API设计
#### GET /assets/realms_icon.svg
- **响应**: `200 image/svg+xml`（SVG 文件内容）

#### GET /favicon.ico
- **响应**: `308 Permanent Redirect` → `/assets/realms_icon.svg`

### 数据模型
（无）

---

## 4. 核心场景

> 执行完成后同步到对应模块文档

### 场景: 全站展示品牌图标（Web 控制台 / 管理后台）
**模块**: `internal/web` / `internal/admin` / `internal/server`
**条件**: 用户访问任意页面（含登录/注册）
**行为**:
- 页面 `<head>` 引用 `/assets/realms_icon.svg` 作为 favicon
- 页面导航区域使用 `/assets/realms_icon.svg` 作为品牌图标
**结果**: UI 全站统一展示 Realms 品牌图标

---

## 5. 技术决策

> 本方案涉及的技术决策，归档后成为决策的唯一完整记录

### ui-icon#D001: 图标存放与分发方式
**日期**: 2026-01-22
**状态**: ✅采纳
**背景**: 需要全站统一使用同一份图标资源，同时避免运行时依赖文件路径，并提供稳定 URL 供模板引用。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 直接在模板内 inline（data URI / inline svg） | 无需路由 | 模板冗长，不利于复用与缓存；多处重复引用 |
| B: `go:embed` + 统一 `/assets/*` 路由（本次采用） | 单一资源源、可缓存、模板简洁、无需运行时文件 | 需要新增路由与测试 |
**决策**: 选择方案 B
**理由**: 资源可复用且易于缓存；与当前 SSR 模板/单体服务形态契合；不引入额外构建链路。
**影响**: `internal/assets`、`internal/server`、`internal/web`、`internal/admin`
