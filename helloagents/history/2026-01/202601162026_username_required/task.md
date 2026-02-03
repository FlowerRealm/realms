# 任务清单: username 改为必填

目录: `helloagents/plan/202601162026_username_required/`

---

## 1. 数据库迁移
- [√] 1.1 新增迁移：回填 `users.username` 的 `NULL/空串` 并设置 `NOT NULL`

## 2. 后端逻辑
- [√] 2.1 `NormalizeUsername`：空值改为报错（username 必填）
- [√] 2.2 用户创建/更新：`CreateUser/UpdateUserUsername` 仅接受必填 username
- [√] 2.3 Web 注册/账号设置/管理后台：移除“可选”分支并保持占用校验

## 3. 前端与文档
- [√] 3.1 模板：注册/账号设置/管理后台用户表单将账号名改为必填 + 文案更新
- [√] 3.2 知识库：更新 `helloagents/wiki/modules/realms.md` 与 `helloagents/CHANGELOG.md`

## 4. 测试
- [√] 4.1 执行 `go test ./...`

