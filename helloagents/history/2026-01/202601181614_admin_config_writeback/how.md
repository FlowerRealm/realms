# 技术设计: 管理后台写回配置文件（UI 配置全部项）

## 技术方案

### 入口与权限

- 新增管理后台页面：
  - `GET /admin/config`：展示 YAML 编辑器
  - `POST /admin/config`：保存（可选重启）
- 通过现有 `adminChain` 限制仅 root + CSRF。

### 配置加载与校验

为避免环境变量覆盖导致“写入的配置文件本身无效但在当前 env 下看似可用”，保存时采用**不带 env 覆盖**的加载校验：

- `internal/config` 新增 `LoadFromFile(path string)`（或等价命名），逻辑与现有 `Load` 一致，但**不应用 env overrides**
- 保存时将用户提交的 YAML 写入临时文件，调用 `config.LoadFromFile(tmp)` 校验：
  - YAML 语法
  - 必填项（`server.addr`、`db.dsn` 等）
  - base_url、时区、duration、范围限制等校验

### 写回策略（原子写入 + 备份）

写回采用以下流程：

1. 在同目录创建临时文件写入内容并 `fsync`
2. 若目标文件存在，先复制/重命名为备份文件（带时间戳）
3. `rename(tmp, target)` 原子替换
4. （可选）`fsync` 目录

### 重启策略

- “保存并重启”会在响应成功后异步发送 `SIGTERM` 给自身：
  - 主进程已监听 `SIGTERM`，会优雅停机
  - 由 systemd/docker restart policy 负责拉起新进程

### UI 形态

采用最小可用 UI（YAML 文本编辑）以覆盖“全部项”：

- textarea 显示当前 config 文件内容
- 两个按钮：保存 / 保存并重启
- 顶部提示：敏感信息风险、多实例限制、重启依赖 supervisor

## 测试与验证

- `go test ./...`
- 手动验证（本地）：
  - `/admin/config` 能读取当前配置文件
  - 保存非法 YAML/非法字段会被拒绝
  - 保存并重启会触发服务退出（依赖外部 supervisor 拉起）

