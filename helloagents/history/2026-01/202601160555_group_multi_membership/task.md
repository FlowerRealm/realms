# 任务清单: Group（用户多组）与组内渠道约束

目录: `helloagents/plan/202601160555_group_multi_membership/`

---

## 1. 数据与存储层
- [√] 1.1 新增 `internal/store/migrations/0022_user_groups_and_plan_group.sql`：创建 `user_groups`、回填 default 与旧 channel_group、迁移订阅套餐字段
- [√] 1.2 在 `internal/store` 增加用户组读写：查询用户组集合、更新用户组集合、按组统计用户
- [√] 1.3 更新用户查询/鉴权相关 SQL：不再依赖 `users.channel_group`，改为读取 `user_groups`

## 2. 鉴权与调度约束
- [√] 2.1 更新 `internal/auth`、`internal/middleware`：Principal 携带用户组集合（并集）
- [√] 2.2 更新 `internal/scheduler`：Constraints 支持 AllowGroups（交集判定），并更新 binding 约束
- [√] 2.3 更新 `internal/api/openai/handler.go`：约束匹配逻辑按用户组集合生效

## 3. Web 与管理台
- [√] 3.1 更新 `/admin/users`：用户组多选（checkbox），保存时强制包含 default
- [√] 3.2 更新 `/admin/subscriptions`：套餐字段从 channel_group 迁移为 group_name（展示/编辑/校验）
- [√] 3.3 更新 `/subscription`：套餐列表按用户组过滤；购买时服务端校验套餐组属于用户

## 4. 安全检查
- [√] 4.1 执行安全检查（输入验证、权限控制、避免绕过购买与调度约束）

## 5. 测试
- [√] 5.1 更新并补齐调度与 handler 单测：组交集约束与绑定不绕过
- [√] 5.2 运行 `go test ./...`

## 6. 文档更新（知识库）
- [√] 6.1 更新 `helloagents/wiki/data.md`：将 channel_group 单字段升级为 user_groups + plan.group_name
- [√] 6.2 更新 `helloagents/wiki/modules/realms.md`：同步“渠道分组/组”口径与管理台入口描述
