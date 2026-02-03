# 任务清单: 渠道绑定模型（channel_models）

目录: `helloagents/plan/202601141714_channel_models_binding/`

---

## 1. 数据库迁移（schema）
- [√] 1.1 新增迁移创建 `channel_models` 表
- [√] 1.2 调整 `managed_models`：上游相关字段不再作为必填/SSOT（必要时迁移为可空）

## 2. Store
- [√] 2.1 调整 `managed_models` 的 CRUD 仅覆盖元信息字段
- [√] 2.2 新增 `channel_models` CRUD（按 channel/public 查询、创建、更新、删除）
- [√] 2.3 新增查询：列出“至少存在一个可用绑定”的启用模型集合（供 `/v1/models` 与 `/models`）

## 3. Scheduler
- [√] 3.1 约束支持“允许 channel 集合”，并更新单测

## 4. OpenAI Handler
- [√] 4.1 请求按 channel_models 计算允许渠道集合并调度
- [√] 4.2 选中渠道后使用该渠道 upstream_model 改写 payload `model`
- [√] 4.3 `GET /v1/models` 改为输出“存在可用绑定”的模型列表

## 5. Admin/Web UI
- [√] 5.1 `/admin/models` 改为管理模型元信息（不再管理绑定）
- [√] 5.2 新增 `/admin/channels/{id}/models` 管理该渠道绑定模型
- [√] 5.3 `/models` 改为展示“存在可用绑定”的模型列表

## 6. 文档与测试
- [√] 6.1 更新 `helloagents/wiki/data.md`、`helloagents/wiki/api.md`、`helloagents/CHANGELOG.md`
- [√] 6.2 执行 `go test ./...`
