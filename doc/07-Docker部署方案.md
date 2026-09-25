# 07 · Docker 部署方案

> 部署形态始终是**一个容器、一个进程、一个端口**：前端构建产物经 `go:embed` 打进二进制（前后端分离见 02 文档第 5 节，对齐 new-api），不存在"前端容器 + 后端容器"的组合，也不引入 nginx 容器。

## 1. 镜像构建（多阶段单镜像）

```dockerfile
# ---------- 阶段 1：构建前端 ----------
FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npm run build            # 产出 /src/web/dist

# ---------- 阶段 2：构建后端（纯 Go，无 CGO） ----------
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 将阶段 1 产物拷入 web/dist，供 main.go 的 //go:embed 打进二进制（new-api 同款）
COPY --from=frontend /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/beacontower .

# ---------- 阶段 3：运行镜像 ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 beacon
USER beacon
WORKDIR /app
COPY --from=backend /out/beacontower /app/beacontower
VOLUME ["/app/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/beacontower"]
```

镜像体积预估 < 40MB（alpine + 约 20MB 静态二进制）。`docker-compose.yml` 里只有 `beacontower` 一个 service（HTTPS 交给宿主机反向代理，而非加挂 nginx 容器）。

## 2. docker-compose.yml（最终形态）

```yaml
services:
  beacontower:
    image: beacontower:latest        # 或 ghcr.io/<you>/beacontower
    container_name: beacontower
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"        # 只绑本机回环，走反代对外
    environment:
      - BEACON_MASTER_KEY=${BEACON_MASTER_KEY}   # 32字节hex: openssl rand -hex 32
      - TZ=Asia/Shanghai
      # - BEACON_SETUP_TOKEN=...      # 可选：防初始化接口被抢注（值见容器日志）
    volumes:
      - ./data:/app/data             # SQLite 库 + master.key（如未显式提供主密钥）
    deploy:
      resources:
        limits: { memory: 256M }
    # read_only: true                # 可选加固：仅 /app/data 可写
```

## 3. 环境变量表

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `BEACON_PORT` | `8080` | 监听端口 |
| `BEACON_DATA_DIR` | `/app/data` | 数据目录（SQLite、密钥） |
| `BEACON_MASTER_KEY` | 自动生成 `master.key` | AES-256-GCM 主密钥（32 字节 hex），**推荐显式提供** |
| `BEACON_SETUP_TOKEN` | 空 | 设置后初始化接口需携带此令牌（首次启动打印于日志） |
| `BEACON_COLLECT_INTERVAL` | `10` | 采集间隔秒数（下限 5） |
| `BEACON_SITE_TITLE` | `BeaconTower` | 站点标题 |
| `BEACON_TRUSTED_PROXIES` | 空 | 反代 CIDR 列表，启用后从 `X-Real-IP` 取来源 |
| `BEACON_GEOIP_CITY` / `BEACON_GEOIP_COUNTRY` | 空（降级在线回显） | 离线 IP 库路径（GeoLite2 City/Country mmdb）；缺省时亦可放入数据目录 `geolite2-city.mmdb` / `geolite2-country.mmdb` 自动载入 |

## 4. HTTPS 反向代理（推荐 Caddy，自动证书）

```caddy
monitor.example.com {
    reverse_proxy 127.0.0.1:8080
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
    }
}
```

Nginx 等价配置要点：`proxy_pass http://127.0.0.1:8080`、SSE 需 `proxy_buffering off` + `proxy_read_timeout 3600s`、传 `X-Real-IP`。

> 容器端口绑定 `127.0.0.1` 是刻意设计：裸端口 8080 只有本机网络可见，外部访问必须经过反代（TLS 终结点），防止配置遗漏导致明文暴露。

## 5. 备份与升级

**备份**（crontab 示例，每日 03:30）：

```sh
sqlite3 /opt/beacontower/data/beacontower.db ".backup /backup/beacon-$(date +%F).db"
# master.key 单独加密存放（如未用环境变量提供主密钥）
```

**升级**：

```sh
docker compose pull && docker compose up -d   # 数据卷不受影响；SQLite 自动跑增量迁移
```

**恢复**：停容器 → 用备份的 `beacontower.db`（和 `master.key`）替换 `data/` 内同名文件 → 启动。

## 6. 健康检查与日志

- `/healthz`：无鉴权健康检查（仅返回 ok，Docker HEALTHCHECK 用）；
- 日志输出 stdout（JSON 可选），`docker logs beacontower` 查看；SSH 连接失败、初始化令牌等均打日志。
