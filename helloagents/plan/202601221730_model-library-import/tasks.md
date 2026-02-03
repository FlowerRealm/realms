# 任务清单: model-library-import

目录: `helloagents/plan/202601221730_model-library-import/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 13
已完成: 10
完成率: 77%
```

---

## 任务列表

### 0. 需求确认（阻断）

- [√] 0.1 确认“模型库数据源”选择（推荐：models.dev；备选：LiteLLM）
  - 验证: 给出将 `model_id` 映射到数据源 key 的规则（是否需要 provider 前缀）

- [√] 0.2 确认字段映射与单位（usd_per_1m vs cost_per_token；icon 是否需要单独字段）
  - 依赖: 0.1

### 1. 配置与客户端（internal/config + internal/modellibrary）

- [-] 1.1 增加模型库配置：`ModelLibraryConfig` + env 覆盖（base_url/api_key/timeout）
  - 文件: `internal/config/config.go`, `internal/config/env_overrides.go`, `.env.example`
  - 依赖: 0.1
  - 验证: `go test ./...`（含 config 测试补充）

- [√] 1.2 实现 Model Library Client：按 `model_id` 查询并返回结构化结果
  - 文件: `internal/modellibrary/modelsdev.go`（新建）
  - 依赖: 1.1
  - 验证: 单测覆盖：成功/404/401/超时/响应格式错误

- [√] 1.3 为 Client 编写单元测试（httptest 远程服务）
  - 文件: `internal/modellibrary/modelsdev_test.go`（新建）
  - 依赖: 1.2

### 2. 管理后台“新增模型字段填充”闭环（internal/admin + internal/server + templates）

- [√] 2.1 增加查询接口：`POST /admin/models/library-lookup`
  - 文件: `internal/admin/model_library_lookup.go`（新建）
  - 行为: 调用 Client → 解析价格/owned_by → 返回 JSON（不写库）
  - 依赖: 1.2
  - 验证: 单测覆盖“解析与返回数据”（不依赖真实 DB）

- [√] 2.2 注册路由并接入 feature gate（与 models 同开关）
  - 文件: `internal/server/app.go`
  - 依赖: 2.1
  - 验证: 访问路由，禁用 models feature 时返回 404

- [√] 2.3 更新 `/admin/models` UI：在“新增模型”弹窗内新增“从模型库查询并填充”区块
  - 文件: `internal/admin/templates/models.html`
  - 交互: 输入 `model_id` → 点击“查询” → 自动填充 `public_id/owned_by/input/output/cache`
  - 依赖: 2.1
  - 验证: 前端可触发 lookup 请求并将返回值写入表单；失败时展示错误提示

- [-] 2.4 复用现有价格解析规则（支持 usd_per_1m + 可选 cost_per_token）
  - 文件: `internal/admin/model_pricing_import.go`（复用/抽取解析函数）+ `internal/admin/model_library_lookup.go`
  - 依赖: 0.2
  - 验证: 单测覆盖 cost_per_token → usd_per_1m 的换算与精度截断

### 4. 文档与验收（docs + tests + KB）

- [-] 4.1 更新 `.env.example`：补充模型库相关变量说明（脱敏）
  - 文件: `.env.example`
  - 依赖: 1.1

- [√] 4.2 更新 `README.md`：补充“模型库字段填充”使用说明（入口/行为/注意事项）
  - 文件: `README.md`
  - 依赖: 2.3

- [√] 4.3 运行测试：`go test ./...`
  - 依赖: 1.3, 2.1, 2.4

- [√] 4.4 知识库同步：更新模块文档与 `helloagents/CHANGELOG.md`
  - 文件: `helloagents/wiki/modules/realms.md`（或新增 `helloagents/modules/*.md` 按知识库规范补齐）
  - 依赖: 4.3

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
