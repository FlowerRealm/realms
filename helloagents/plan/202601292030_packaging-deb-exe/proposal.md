# 变更提案: packaging_deb_exe

## 元信息
```yaml
类型: 构建/发布
方案类型: implementation
优先级: P1
状态: ✅完成
创建: 2026-01-29
```

---

## 1. 需求

### 背景
当前部署方式以 Docker 为主。为方便用户在无 Docker / 有传统运维流程的环境使用，需要提供可直接安装/运行的发布产物。

### 目标
- 提供 Debian/Ubuntu 可安装的 `.deb` 包
- 提供 Windows 可直接运行的 `realms.exe`（以 zip 方式分发）
- 提供 macOS 可下载运行的产物（darwin/amd64、darwin/arm64，以 tar.gz 分发）
- tag 发布时自动构建并上传到 GitHub Release

### 约束条件
- Realms 为 Go 单体服务（静态编译，`CGO_ENABLED=0`）
- 默认数据库可用 SQLite（适合 `.deb` 单机部署默认配置）
- 产物构建应包含版本信息注入（`realms/internal/version.Version/Date`）

### 验收标准
- [ ] push tag 后，GitHub Release 附带 `.deb` / Windows zip / Linux tar.gz / macOS tar.gz
- [ ] `.deb` 安装后默认以 systemd 服务启动，配置文件为 `/etc/realms/realms.env`
- [ ] 文档包含非 Docker 安装方式说明

---

## 2. 方案

### 技术方案概览
1) 增加 deb 打包文件：
   - `packaging/debian/realms.service`（systemd unit）
   - `packaging/debian/realms.env`（默认配置）
   - `packaging/debian/postinst` / `prerm`（用户/目录创建、服务启停）
2) 增加构建脚本：
   - `scripts/build-deb.sh`：构建 `.deb`
   - `scripts/build-release.sh`：构建 linux tar.gz / macOS tar.gz / windows zip / deb
3) 增加 GitHub Actions：
   - `.github/workflows/release.yml`：tag push 触发，构建并上传 Release 附件
4) 文档同步：
   - `README.md`、`docs/USAGE.md` 增加安装方式说明

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| `.deb` 安装环境无 systemd | 中 | postinst 对 `systemctl` 做存在性检查，失败不阻断安装 |
| Windows 服务化诉求 | 中 | 默认提供可运行二进制；服务化建议使用外部工具（如 NSSM）或后续专门实现 |
