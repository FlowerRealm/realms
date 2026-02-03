# 任务清单: packaging_deb_exe

目录: `helloagents/plan/202601292030_packaging-deb-exe/`

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
总任务: 6
已完成: 6
完成率: 100%
```

---

## 任务列表

### 1. 打包文件（Debian）

- [√] 1.1 增加 systemd unit 与默认 env
  - 文件: `packaging/debian/realms.service`、`packaging/debian/realms.env`

- [√] 1.2 增加 deb 安装/卸载脚本
  - 文件: `packaging/debian/postinst`、`packaging/debian/prerm`

### 2. 构建脚本

- [√] 2.1 增加 deb 构建脚本
  - 文件: `scripts/build-deb.sh`

- [√] 2.2 增加发布产物构建脚本（linux tar.gz / macOS tar.gz / windows zip / deb）
  - 文件: `scripts/build-release.sh`

### 3. GitHub Actions

- [√] 3.1 tag push 自动构建并上传 GitHub Release
  - 文件: `.github/workflows/release.yml`

### 4. 文档

- [√] 4.1 README/部署文档补齐非 Docker 安装方式
  - 文件: `README.md`、`docs/USAGE.md`

- [√] 4.2 知识库模块文档补齐发布产物说明
  - 文件: `helloagents/modules/_index.md`、`helloagents/modules/packaging_installers.md`

- [√] 4.3 更新 Changelog
  - 文件: `helloagents/CHANGELOG.md`
