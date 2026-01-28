# 版本与更新

## 运行时版本（实例自身）

Realms 在运行时提供两个公开接口用于查询“当前实例的构建指纹”：

- `GET /api/version`
- `GET /healthz`

它们会返回：

- `version`：构建版本（release 时建议注入为 git tag；本地开发默认 `dev`）
- `date`：构建时间（可注入）

## 最新发布版本（latest）

本项目会在 GitHub Pages 站点根目录提供 `version.json`（作为 latest 发布版本号的 SSOT），用于外部查询与升级提示：

- `https://<owner>.github.io/<repo>/version.json`

该文件由 GitHub Actions 在部署文档站点时生成（tag push / master push），字段包含：

- `latest`
- `released_at`
- `repo`
- `docs`
- `docker_image`

> 说明：按当前约束，对外不提供任何 sha/commit 字段。

## 启用 GitHub Pages（仓库设置）

首次启用需要在 GitHub 仓库设置中将 Pages 的发布源切换为 GitHub Actions：

1) 打开仓库 Settings → Pages  
2) 在 “Build and deployment” 中选择 Source = **GitHub Actions**  
3) push 到 `master`（或打 tag）后，`pages` workflow 会自动部署文档站点与 `version.json`

## Docker 镜像版本

当你使用官方 Docker 镜像时，推荐直接使用 tag 作为版本：

- `flowerrealm/realms:<tag>`

并可在运行实例中通过 `/api/version` 校验镜像构建信息是否与预期一致。
