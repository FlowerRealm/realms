# 任务清单: SSR 模板渲染修复（Content 注入）

目录: `helloagents/plan/202601140625_template_content_render/`

---

## 1. Web 控制台模板修复
- [√] 1.1 修复 `internal/web/templates/base.html` 中的动态模板名用法，避免启动时报 `unexpected ".Content" in template clause`
- [√] 1.2 调整 `internal/web/server.go` 渲染流程：先渲染页面内容模板，再注入到 base 模板中

## 2. Admin 模板修复
- [√] 2.1 修复 `internal/admin/templates/base.html` 同类问题
- [√] 2.2 调整 `internal/admin/server.go` 渲染流程与数据结构

## 3. Store 启动修复
- [√] 3.1 修复 `groups` 表名关键字冲突：在 `internal/store/store.go` 中对 SQL 表名使用反引号，避免启动时语法错误

## 4. 文档与记录
- [√] 4.1 更新 `helloagents/wiki/modules/codex.md`：补充本次变更记录
- [√] 4.2 更新 `helloagents/CHANGELOG.md`（Unreleased）

## 5. 验证
- [√] 5.1 运行 `go test ./...`
- [√] 5.2 使用本地 MySQL 启动一次 `go run ./cmd/codex -config config.yaml`，验证不再因模板解析失败退出

## 6. 迁移
- [√] 6.1 迁移方案包至 `helloagents/history/2026-01/202601140625_template_content_render/` 并更新 `helloagents/history/index.md`
