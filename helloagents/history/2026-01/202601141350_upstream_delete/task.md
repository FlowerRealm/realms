# 任务清单: 管理后台上游硬删除

目录: `helloagents/history/2026-01/202601141350_upstream_delete/`

---

## 1. Store
- [√] 1.1 新增上游资源硬删除方法（物理删除 + 事务级联）
- [√] 1.2 补充按 id 查询 credential/account 的方法（用于权限校验与重定向）

## 2. Admin 路由与页面
- [√] 2.1 新增删除 Handler 与路由（channel/endpoint/credential/account）
- [√] 2.2 管理后台模板增加删除按钮与 status 展示

## 3. 文档更新
- [√] 3.1 更新 `helloagents/wiki/api.md`
- [√] 3.2 更新 `helloagents/CHANGELOG.md` 与 `helloagents/history/index.md`

## 4. 测试
- [√] 4.1 `go test ./...`
