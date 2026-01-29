# Realms 部署指南

本文只提供“可直接复制运行”的部署命令。  
环境变量与 `.env` 的详细配置项说明请先阅读：[`README.md`](https://github.com/FlowerRealm/realms/blob/master/README.md) 与 [`.env.example`](https://github.com/FlowerRealm/realms/blob/master/.env.example)。

## 方式 0：安装包（Deb/Windows/macOS）

从 GitHub Releases 下载对应版本的安装包/二进制产物（建议使用 tag，例如 `v1.2.3`）：
- Debian/Ubuntu：`realms_<TAG>_linux_amd64.deb` / `realms_<TAG>_linux_arm64.deb`
- Windows：`realms_<TAG>_windows_amd64.zip`（内含 `realms.exe`）
- macOS：`realms_<TAG>_darwin_arm64.tar.gz`（Apple Silicon）/ `realms_<TAG>_darwin_amd64.tar.gz`（Intel）

> 说明：`.deb` 默认按 SQLite（单机）配置并以 systemd 服务启动；Docker Compose 依然是推荐方式（尤其是需要 MySQL/更复杂依赖时）。

### Debian/Ubuntu（.deb，systemd）

```bash
sudo dpkg -i "./realms_<TAG>_linux_amd64.deb"

# 默认监听 :8080；修改配置后重启服务
sudoedit "/etc/realms/realms.env"
sudo systemctl restart "realms"

# 验证
curl -fsS "http://127.0.0.1:8080/healthz"
```

### Windows（zip/realms.exe）

1) 解压 `realms_<TAG>_windows_amd64.zip`  
2) 在解压目录执行（或手动复制/重命名）：

```powershell
Copy-Item ".env.example" ".env"
.\realms.exe
```

### macOS（tar.gz）

```bash
tar -xzf "./realms_<TAG>_darwin_arm64.tar.gz"
cd "realms_<TAG>_darwin_arm64"

# 安装到 PATH（示例：/usr/local/bin）
sudo install -m 0755 "./realms" "/usr/local/bin/realms"

cp ".env.example" ".env"
realms
```

## 从 0 开始（Docker Compose，一键）

```bash
git clone "https://github.com/FlowerRealm/realms.git"
cd "realms"

cp ".env.example" ".env"
docker compose pull realms
docker compose up -d

curl -fsS "http://127.0.0.1:18080/healthz"
```

首次初始化（必须做一次）：

1) 打开：`http://127.0.0.1:18080/`  
2) 注册并登录（`REALMS_ALLOW_OPEN_REGISTRATION=true` 时允许注册）  
3) **第一个注册用户会成为 `root`**  
4) 初始化完成后建议把 `REALMS_ALLOW_OPEN_REGISTRATION` 改为 `false`，并重启：

```bash
docker compose up -d
```

## 方式 A：Docker Compose（推荐，单机一键）

### 1) 准备 `.env`

```bash
cp .env.example .env
```

如果你只想“无脑启动”，也可以用最小 `.env`（示例密码为 `root`，请自行修改）：

```bash
cat > .env <<'EOF'
MYSQL_ROOT_PASSWORD=root
MYSQL_DATABASE=realms

REALMS_ENV=prod
REALMS_ADDR=:8080
REALMS_DB_DSN_DOCKER=root:root@tcp(mysql:3306)/realms?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&time_zone=%27%2B00%3A00%27

REALMS_ALLOW_OPEN_REGISTRATION=true
REALMS_DISABLE_SECURE_COOKIES=true
EOF
```

然后（按需）编辑 `.env`（至少确认/修改）：
- `MYSQL_ROOT_PASSWORD` / `MYSQL_DATABASE`
- `MYSQL_HOST_PORT`（可选：MySQL 暴露到宿主机的端口，默认 `3306`）
- `MYSQL_BIND_IP`（可选：MySQL 绑定地址；默认 `0.0.0.0` 对外监听；如不需要对外访问建议改为 `127.0.0.1`）
- `REALMS_IMAGE`（可选：默认 `flowerrealm/realms`，如需固定版本请写 `flowerrealm/realms:<tag>`）
- `REALMS_ALLOW_OPEN_REGISTRATION`（建议仅初始化阶段临时开启）
- `REALMS_DISABLE_SECURE_COOKIES`（纯 HTTP 场景一般需要 `true`）

> 如果你改了 `MYSQL_ROOT_PASSWORD` 或 `MYSQL_DATABASE`，请同步修改 `REALMS_DB_DSN_DOCKER` 里的用户名/密码/库名。
>
> ⚠️ 注意：对外暴露 MySQL 端口有安全风险。公网部署请务必通过防火墙限制来源 IP、关闭 root 远程登录/使用最小权限账号，并设置强密码。

### 2) 启动（后台）

```bash
docker compose pull realms
docker compose up -d
```

### 3) 查看状态 / 日志

```bash
docker compose ps
docker compose logs -f realms
```

### 4) 停止

```bash
docker compose down
```

### 5) 验证服务可用

```bash
curl -fsS "http://127.0.0.1:18080/healthz"
```

### 6) 首次初始化（必须做一次）

1) 打开：`http://127.0.0.1:18080/`  
2) 注册并登录（`REALMS_ALLOW_OPEN_REGISTRATION=true` 时允许注册）  
3) **第一个注册用户会成为 `root`**  
4) 初始化完成后，建议把 `REALMS_ALLOW_OPEN_REGISTRATION` 改为 `false`，并重启：

```bash
docker compose up -d
```

## 日常维护（Docker Compose）

### 更新/升级到最新版

```bash
cd "realms"
git checkout "master" # 如果你的默认分支是 main，请改为 main
git pull --rebase
docker compose pull realms
docker compose up -d
docker compose logs -f realms
```

### 回滚到某个版本

> 注意：如果新版本已经执行了数据库迁移，回滚代码不一定能兼容当前库结构。稳妥做法是先备份数据库（见下方），必要时恢复备份。

```bash
cd "realms"
# 通过镜像 tag 回滚（将 <TAG> 替换为目标版本；如 v1.2.3）。
#
# REALMS_IMAGE 会被 docker-compose.yml 读取：默认是 flowerrealm/realms。
# 你也可以直接写入 .env：REALMS_IMAGE=flowerrealm/realms:<TAG>
export REALMS_IMAGE="flowerrealm/realms:<TAG>"
docker compose pull realms
docker compose up -d
docker compose logs -f realms
```

### 备份数据库（导出 SQL）

```bash
cd "realms"
docker compose exec -T mysql sh -lc 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --databases "$MYSQL_DATABASE" --single-transaction --quick --set-gtid-purged=OFF' \
  > "backup.sql"
```

### 恢复数据库（从 SQL 导入）

> ⚠️ 会覆盖同名数据库内容。建议先停止 `realms` 再导入。

```bash
cd "realms"
docker compose stop realms
cat "backup.sql" | docker compose exec -T mysql sh -lc 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD"'
docker compose start realms
```

### 迁移到新机器（推荐：SQL 备份迁移）

在旧机器上：

```bash
cd "realms"
docker compose exec -T mysql sh -lc 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --databases "$MYSQL_DATABASE" --single-transaction --quick --set-gtid-purged=OFF' \
  > "backup.sql"
cp ".env" ".env.backup"
```

把 `backup.sql` 与 `.env.backup` 传到新机器（示例用 scp）：

```bash
scp "backup.sql" "<USER>@<NEW_HOST>:~/backup.sql"
scp ".env.backup" "<USER>@<NEW_HOST>:~/realms.env"
```

在新机器上：

```bash
git clone "https://github.com/FlowerRealm/realms.git"
cd "realms"

cp "~/realms.env" ".env"
docker compose up -d mysql
until docker compose exec -T mysql sh -lc 'mysqladmin ping -h 127.0.0.1 -uroot -p"$MYSQL_ROOT_PASSWORD" --silent'; do sleep 1; done

cat "~/backup.sql" | docker compose exec -T mysql sh -lc 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD"'
docker compose pull realms
docker compose up -d realms
```

## 方式 B：Docker（仅 Realms，外部 MySQL）

适合你已经有可连接的 MySQL（云数据库/外部主机），不想用本仓库的 MySQL 容器。

### 1) 构建镜像

```bash
docker build -t realms:local .
```

### 2) 准备 `.env`（容器内由 Realms 自己加载）

```bash
cp .env.example .env
```

编辑 `.env`：
- 把 `REALMS_DB_DSN` 改成你的 MySQL 连接（注意：容器内不能用 `127.0.0.1` 直连宿主机 MySQL）

### 3) 运行容器（挂载 `.env`）

```bash
docker run -d --name realms \
  -p 8080:8080 \
  -v "$(pwd)/.env:/.env:ro" \
  realms:local
```

查看日志 / 停止：

```bash
docker logs -f realms
docker stop realms
docker rm -f realms
```

## 方式 C：二进制（直接运行）

如果你从 Release 获取了二进制：把文件名替换成 `./realms` 即可。  
如果你在仓库内自行构建：

```bash
go build -o realms ./cmd/realms
cp .env.example .env
./realms
```
