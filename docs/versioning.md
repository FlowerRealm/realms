# 版本与更新

## 运行时版本（实例自身）

Realms 在运行时提供公开接口用于查询“当前实例的构建指纹”：

- `GET /healthz`

它会返回：

- `version`：构建版本（release 时建议注入为 git tag；本地开发默认 `dev`）
- `date`：构建时间（可注入）

## 最新发布版本（latest）

仓库不再通过 GitHub Actions 自动发布 GitHub Pages 文档站点与 `version.json`（因此也不再提供对外可查询的 latest 版本号 SSOT）。

如需对外提供“latest”查询能力，建议选择其一：

- 以 **Git tag** 作为发布版本号（消费者侧通过 GitHub API/clone 获取最新 tag）
- 以 **Docker 镜像 tag** 作为发布版本号（例如 `flowerrealm/realms:<tag>`）
- 在你自己的制品/站点中维护 `version.json`（字段结构可按需自定义）

## Docker 镜像版本

当你使用官方 Docker 镜像时，推荐直接使用 tag 作为版本：

- `flowerrealm/realms:<tag>`

并可在运行实例中通过 `/healthz` 校验镜像构建信息是否与预期一致。
