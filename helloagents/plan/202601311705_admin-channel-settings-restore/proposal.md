# 变更提案：恢复上游渠道“设置项”完整性（配置收敛到设置弹窗）

目标：以旧版 SSR `internal/admin/templates/channels.html` 为基线，修复 SPA 管理后台 `/admin/channels` 的“设置项缺失/分散”问题，将所有渠道配置统一收敛到“设置”弹窗中（不再拆成独立弹窗/独立入口），并保持全站中文与 Bootstrap 小窗交互一致。

---

## 页面对照计划（按页面）

### 页面：`/admin/channels`（上游渠道管理）

#### 原来的内容（SSR）

- 顶部：标题 + 新建渠道按钮；统计区间（开始/结束日期）表单（含时区提示）
- 列表：拖拽排序；每行展示渠道基础信息、用量统计、运行态健康信息
- 交互：所有“配置项”集中在 **同一个渠道设置弹窗** `channelSettingsModal-*` 内，主要分区：
  - 常用设置（base_url 等）
  - 密钥管理（凭证列表 + 添加 + 删除；仅展示提示）
  - 模型绑定（旧版：列表 + 新增 + 更新 + 删除；本次 SPA 目标态：模型选择 + 模型重定向）
  - 分组设置（checkbox 多选）
  - 渠道属性（组织 ID/默认测试模型/Tag/权重/自动封禁/备注）
  - 请求处理设置（推理内容合并/透传请求体/代理/系统提示词/系统提示词覆盖）
  - 请求字段策略（allow_service_tier/disable_store/allow_safety_identifier）
  - 参数改写/Header Override/模型后缀保护名单/请求体黑白名单/状态码映射
  - 危险操作（删除渠道）

#### 现在的内容（SPA 迁移后问题态）

- “设置”弹窗只包含少量基础字段（name/status/base_url/groups/priority/promotion + 追加 key + request policy）
- **模型绑定、查看 Key 被拆成独立弹窗**（配置入口分散，违反“配置项全部放到设置中”的要求）
- 缺失大量高级设置项（渠道属性/请求处理设置、param_override/header_override/body filters/status_code_mapping 等）
- 密钥管理缺失：无法看到凭证列表/删除/按渠道管理（仅能“查看 Key”）
- 分组设置为纯文本 CSV，缺少旧版 checkbox 交互与禁用态提示

#### 差异（需要修复的点）

1. 配置分散：模型绑定/Key 相关不应独立成弹窗或独立入口，应收敛到“设置”中
2. 设置项缺失：需要补齐 SSR settings modal 的全部分区（至少覆盖“渠道属性/请求处理设置” + overrides/filters/mapping）
3. 密钥管理退化：需要恢复“凭证列表 + 添加 + 删除”，并保留可选的明文 Key 查看能力（root）
4. 分组设置退化：需要恢复 checkbox 多选（并提示禁用分组状态）
5. 一致性：全中文、Bootstrap modal 小窗交互、样式密度对齐 SSR
6. 模型绑定体验：配置模型时只做“选择”，并在下方提供“模型重定向”（对外模型 → 上游模型），避免逐行增删改造成的复杂度与误操作

---

## 实施摘要（本次实现方式）

- 将 SPA 的 `channelModelsModal` / `channelKeyModal` 删除，把相关能力迁入 `editChannelModal`（tab：常用/密钥/模型绑定/高级）
- 新增/补齐后端 JSON API（root session）以支持：
  - 渠道凭证列表/添加/删除
  - 渠道属性/请求处理设置更新
  - param_override/header_override/model_suffix_preserve/request_body_* / status_code_mapping 更新
- 模型绑定在 SPA 侧改为“模型选择 + 模型重定向”：上方勾选“允许模型”，下方维护“模型重定向”，保存时仅更新/创建 `channel_models` 记录（`status/upstream_model`）
- `GET /api/channel/:id` 扩展为返回完整 settings 字段（供设置弹窗读取/回填）
