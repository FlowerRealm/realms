# realms (npm)

通过 npm 全局安装并运行 Realms 的 personal App（`realms-app`）。

## 安装

```bash
npm install -g realms
```

安装完成后会自动下载对应平台的 `realms-app`（来自 GitHub Releases），并提供命令 `realms`。

## 运行

```bash
realms
```

默认行为与 `realms-app` 一致（personal 模式、SQLite、本地启动后打开浏览器到 `/login`）。

## 环境变量

- `REALMS_APP_BASE_URL`：下载源前缀（默认：`https://github.com/FlowerRealm/realms/releases/download`）
- `REALMS_APP_VERSION`：强制下载的版本（例如 `v0.16.0`）
- `REALMS_APP_SKIP_DOWNLOAD=1`：跳过下载（你自行放置二进制）
- `REALMS_APP_BIN`：直接指定要运行的 `realms-app` 路径（跳过下载与 vendor 查找）
- `REALMS_APP_SKIP_CHECKSUM=1`：跳过 sha256 校验（不推荐）

## 故障排查

1) 如果你安装时用了 `--ignore-scripts`（或环境禁用了 scripts），postinstall 不会执行：请重新安装或手动运行：

```bash
node "$(npm root -g)/realms/scripts/postinstall.js"
```

2) 下载失败：可配置 `REALMS_APP_BASE_URL` 指向镜像/CDN，并重试安装。

