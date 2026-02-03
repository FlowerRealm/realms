# 变更提案: remove-footer-version

## 元信息
```yaml
类型: 优化
方案类型: implementation
优先级: P1
状态: 草稿
创建: 2026-02-01
```

---

## 1. 需求

### 背景
当前前端页脚（`ProjectFooter`）会额外拉取并展示构建版本号（`GET /api/version`）。用户明确希望移除“前端显示版本号”的功能，且不需要保留 version 相关能力。

### 目标
- 前端页脚不再展示版本号（用户侧 / 管理侧 / 公共页均生效）
- 前端不再请求 `/api/version`
- 后端移除 `/api/version` 接口（避免遗留的“公开构建信息”入口）
- 同步更新文档与知识库中关于 `/api/version`、页脚版本展示的说明

### 约束条件
```yaml
时间约束: 无
性能约束: 无（预期性能更好：少一次请求）
兼容性约束: 不做向后兼容（按需求直接移除 /api/version）
业务约束: 仅移除“版本号展示/查询”能力，不改变其他页面与 API 行为
```

### 验收标准
- [ ] `web/src/layout/ProjectFooter.tsx` 不再渲染版本号（DOM 不包含 `#rlmBuildInfo`）
- [ ] SPA 页面加载不再发起 `/api/version` 请求
- [ ] 后端不再注册 `/api/version` 路由；请求该路径返回 404
- [ ] `go test ./...` 与 `npm --prefix web run build` 通过
- [ ] 仓库文档与 helloagents 知识库中不再出现“/api/version 用于页脚版本展示”的描述（以代码为准）

---

## 2. 方案

### 技术方案
1) 前端：精简 `ProjectFooter`，移除 build info 的 state / fetch / 渲染分支，仅保留固定文案 + 年份 + GitHub 链接。  
2) 后端：删除系统路由 `/api/version`，同时移除对应的 `router.Options.Version` 注入点与 `handleVersion` 实现。  
3) 文档：删除/改写所有 `/api/version` 相关描述（保留 `/healthz` 的健康检查文档）。  

> 备注：当前 `/healthz` 仍会返回 version/date，但前端不依赖；本提案按需求仅移除“页脚版本号展示 + /api/version（UI 专用）”。

### 影响范围
```yaml
涉及模块:
  - web SPA: 移除页脚版本号展示与拉取逻辑
  - router(system): 移除 /api/version 路由
  - server(app): 移除 handleVersion 与 router.Options.Version 注入
  - docs / helloagents KB: 同步删除相关描述
预计变更文件: 8-12
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 外部使用者依赖 `/api/version` | 低 | 仓库内检索确认仅前端使用；同步更新文档并在 PR/发布说明中提示 |
| 页脚分隔符渲染异常（多余 “·”） | 低 | 手动检查 app/admin/public 三种 variant 的页脚渲染 |

---

## 3. 技术设计（可选）

> 涉及架构变更、API设计、数据模型变更时填写

### 架构设计
不涉及架构调整。

### API设计
#### 删除 GET /api/version
- **行为**: 路由不再注册；请求返回 404

### 数据模型
不涉及数据模型变更。

---

## 4. 核心场景

> 执行完成后同步到对应模块文档

### 场景: SPA 页面渲染页脚
**模块**: web SPA（`ProjectFooter`）
**条件**: 用户访问任意页面（public/app/admin）
**行为**: 页脚仅展示固定信息（产品文案 + 年份 + GitHub 链接），不展示构建版本信息
**结果**: UI 侧无版本号露出，且不产生 `/api/version` 请求

---

## 5. 技术决策

> 本方案涉及的技术决策，归档后成为决策的唯一完整记录

### remove-footer-version#D001: 移除 /api/version（与页脚版本展示一起删除）
**日期**: 2026-02-01
**状态**: ✅采纳
**背景**: `/api/version` 当前仅用于前端页脚展示版本号；用户明确不需要保留 version 能力，要求直接删除。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 仅移除前端展示，保留 `/api/version` | 对外仍可查询构建信息 | 继续暴露版本信息，且留下无用接口 |
| B: 同时移除前端展示与 `/api/version`（推荐） | 行为与需求一致；减少信息暴露与维护面 | 可能影响少量依赖该接口的外部脚本（需自行适配） |
**决策**: 选择方案 B
**理由**: 代码检索表明仓库内仅 `ProjectFooter` 依赖 `/api/version`；删除接口更符合“直接删了”的需求，并减少遗留入口。
**影响**: `web/src/layout/ProjectFooter.tsx`、`router/system_routes.go`、`router/options.go`、`internal/server/app.go`、相关文档
