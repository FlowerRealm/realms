# 任务清单：恢复上游渠道“更多设置项”并收敛到设置弹窗

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 对齐 SSR 设置项结构（/admin/channels）

- [√] 盘点 `internal/admin/templates/channels.html` 的 `channelSettingsModal-*` 分区清单
- [√] 将“模型绑定/查看 Key”等配置入口收敛到“设置”弹窗（移除独立弹窗）
- [√] “设置”弹窗采用 tab 组织：常用 / 密钥 / 模型绑定 / 高级（保持小窗交互）

## B. 后端 API 补齐（root session）

- [√] `GET /api/channel/:channel_id`：返回完整渠道 settings 字段（渠道属性/请求处理设置 + overrides/filters/mapping）
- [√] 新增渠道凭证管理 API：`GET/POST/DELETE /api/channel/:channel_id/credentials`
- [√] 新增渠道设置更新 API：
  - [√] `PUT /api/channel/:channel_id/meta`
  - [√] `PUT /api/channel/:channel_id/setting`
  - [√] `PUT /api/channel/:channel_id/param_override`
  - [√] `PUT /api/channel/:channel_id/header_override`
  - [√] `PUT /api/channel/:channel_id/model_suffix_preserve`
  - [√] `PUT /api/channel/:channel_id/request_body_whitelist`
  - [√] `PUT /api/channel/:channel_id/request_body_blacklist`
  - [√] `PUT /api/channel/:channel_id/status_code_mapping`

## C. 前端对齐（SPA）

- [√] `ChannelsPage.tsx`：设置弹窗内实现分区表单与保存动作（含错误提示与刷新）
- [√] 密钥管理：凭证列表 + 新增 + 删除；可选读取明文 key（root）
- [√] 模型绑定：改为“模型选择 + 模型重定向”，保存时同步 `status/upstream_model`（并保留禁用记录，避免反复启用/禁用丢失重定向）
- [√] 分组设置：对齐 SSR（checkbox 多选 + 禁用分组提示）

## D. 验证

- [√] `go test ./...`
- [√] `cd web && npm run lint`
- [√] `cd web && npm run build`

## E. 知识库同步

- [√] `helloagents/modules/web_spa.md`：补充“渠道设置弹窗=配置唯一入口”的约定与 API 映射
- [√] `helloagents/CHANGELOG.md`：记录本次修复
