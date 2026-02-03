# 发布产物与安装包（deb/Windows）

本文记录 Realms 在 **非 Docker** 场景的发布产物形态与构建方式（Debian/Ubuntu `.deb`、Windows `realms.exe`）。

## 产物列表

以 tag `v1.2.3` 为例（产物会上传到 GitHub Release）：

- `realms_v1.2.3_linux_amd64.tar.gz`
- `realms_v1.2.3_linux_arm64.tar.gz`
- `realms_v1.2.3_darwin_amd64.tar.gz`
- `realms_v1.2.3_darwin_arm64.tar.gz`
- `realms_v1.2.3_linux_amd64.deb`
- `realms_v1.2.3_linux_arm64.deb`
- `realms_v1.2.3_windows_amd64.zip`（包含 `realms.exe`、`.env.example`、`README.md`）

## Debian/Ubuntu（.deb）说明

`.deb` 安装后默认以 systemd 服务启动：

- service：`realms.service`
- 配置：`/etc/realms/realms.env`（systemd `EnvironmentFile`）
- 数据：`/var/lib/realms`（SQLite db、工单附件等）

修改配置后：

```bash
sudo systemctl restart realms
```

## 构建方式

### 本地构建发布产物（建议用于校验）

```bash
make release-artifacts VERSION=v1.2.3
```

产物输出到 `dist/`。

### CI 构建（tag push）

当 push tag（例如 `v1.2.3`）时：

- `.github/workflows/release.yml` 会构建产物并上传到 GitHub Release
- `.github/workflows/docker.yml` 会构建并推送 Docker 镜像
