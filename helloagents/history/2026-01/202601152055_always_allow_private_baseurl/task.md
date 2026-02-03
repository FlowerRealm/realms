# 任务清单: always_allow_private_baseurl

目录: `helloagents/plan/202601152055_always_allow_private_baseurl/`

---

## 1. 移除 base_url 地址范围限制逻辑
- [√] 1.1 在 `internal/security/ssrf.go` 简化 `ValidateBaseURL`：仅做协议/Host/DNS 校验，不再因地址范围报错

## 2. 移除相关配置开关
- [√] 2.1 在 `internal/config/config.go` 删除 `ssrf_allow_private_ranges` 配置项与环境变量覆盖逻辑
- [√] 2.2 在 `config.yaml` 与 `config.example.yaml` 删除该配置项，避免用户再次看到/误用

## 3. 管理后台与数据面联动修正
- [√] 3.1 在 `internal/upstream/executor.go` / `internal/admin/server.go` 更新 `ValidateBaseURL` 调用与相关字段/参数
- [√] 3.2 在 `internal/admin/templates/endpoints.html` 移除“可收紧/可拒绝”的提示文案，只保留通用提示
- [√] 3.3 在 `internal/server/app.go` 移除 `/healthz` 的相关字段输出，避免继续暴露/暗示该开关

## 4. 文档同步
- [√] 4.1 在 `README.md` 与 `helloagents/wiki/modules/realms.md` 删除该开关相关描述
- [√] 4.2 在 `helloagents/CHANGELOG.md` 记录变更（不再出现误导结论）

## 5. 测试
- [√] 5.1 执行 `gofmt` 与 `go test ./...`
