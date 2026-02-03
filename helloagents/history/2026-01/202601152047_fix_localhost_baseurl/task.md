# 任务清单: fix_localhost_baseurl

目录: `helloagents/history/2026-01/202601152047_fix_localhost_baseurl/`

---

## 1. 默认配置一致性
- [√] 1.1 调整默认配置：在 `config.yaml` 中将 `security.ssrf_allow_private_ranges` 设为 `true`，确保默认配置与代码默认值/README/知识库一致

## 2. 管理后台提示文案
- [√] 2.1 在 `internal/admin/templates/endpoints.html` 中调整 Base URL 提示文案，避免产生误导结论

## 3. 错误信息与注释
- [√] 3.1 在 `internal/security/ssrf.go` 中移除误导性注释并优化错误信息

## 4. 测试
- [√] 4.1 执行 `go test ./...`，确保构建与测试通过
