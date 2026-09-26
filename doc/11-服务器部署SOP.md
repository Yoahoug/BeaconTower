# 服务器部署 SOP（ops）

> 目标机：`10.66.66.66`（server-ops skill）。镜像由 GitHub Actions 构建发布到 GHCR，
> 服务器只做 pull + compose up，构建不占服务器资源。

## 首次部署（已完成 2026-09-26）

```sh
# 1. 建目录 + 编排 + 密钥
mkdir -p /data/appdata/beacontower/data
# docker-compose.yml 内容见仓库 docker-compose.deploy.yml（见下）
KEY=$(openssl rand -hex 32)
echo "BEACON_MASTER_KEY=$KEY" > /data/appdata/beacontower/.env   # chmod 600

# 2. 修数据目录属主（容器内 uid 10001 非 root；root 属主目录 SQLite 打不开 → error 14）
chown -R 10001:10001 /data/appdata/beacontower/data

# 3. 拉镜像并启动
cd /data/appdata/beacontower && docker compose pull && docker compose up -d

# 4. 验证
docker ps --filter name=beacontower --format "{{.Status}}"   # healthy
curl -s http://127.0.0.1:8091/healthz                         # OK
curl -s http://127.0.0.1:8091/api/v1/public/servers           # 本机节点 online
```

## 日常更新（GitHub 构建 → 服务器拉取）

```sh
# 本地：push 到 main 即自动构建镜像（~2 分钟）
git push origin main
gh run watch --repo Yoahoug/BeaconTower   # 可选：盯构建

# 服务器：拉新镜像 + 滚动重建
cd /data/appdata/beacontower
docker compose pull && docker compose up -d
docker image prune -f                      # 清理悬空旧镜像
```

## docker-compose.yml（/data/appdata/beacontower/）

```yaml
services:
  beacontower:
    image: ghcr.io/yoahoug/beacontower:latest
    container_name: beacontower
    restart: unless-stopped
    ports:
      - "127.0.0.1:8091:8080"     # 8080 被 sub2api 占用 → 宿主用 8091，只绑回环走反代
    environment:
      - BEACON_MASTER_KEY=${BEACON_MASTER_KEY}   # .env 提供，32 字节 hex
      - TZ=Asia/Shanghai
    volumes:
      - ./data:/app/data          # SQLite 库 + master.key
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/healthz"]
      interval: 30s
      timeout: 3s
      retries: 3
```

## 排查记录

| 症状 | 原因 | 处置 |
|---|---|---|
| `打开数据库失败: unable to open database file (14)` 循环重启 | 宿主 `data/` 为 root 属主，容器内 beacon(uid 10001) 不可写 | `chown -R 10001:10001 data/`；新镜像已在构建期 `mkdir + chown /app/data`，匿名卷首挂载继承镜像属主，全新部署不再需要手工 chown |

## 相关入口

- 面板（本机）：http://10.66.66.66:8091（绑回环，需经反代或 SSH 隧道访问）
- GHCR 镜像：`ghcr.io/yoahoug/beacontower:latest`（另含 `${sha}` tag 可回滚）
- 构建日志：GitHub 仓库 → Actions → "Docker image (GHCR)"
