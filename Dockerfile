# ---------- 阶段 1：构建前端 ----------
FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
# npm 缓存挂 BuildKit cache：package-lock 不变时 npm ci 只从本地缓存解包，不走网络
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-fund --no-audit --prefer-offline
COPY web/ ./
RUN npm run build            # 产出 /src/web/dist

# ---------- 阶段 2：构建后端（纯 Go，无 CGO） ----------
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
# 模块缓存挂 BuildKit cache：go.sum 不变时秒级完成（原每次全量下载 ~8s）
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
# 将阶段 1 产物拷入 web/dist，供 main.go 的 //go:embed 打进二进制
COPY --from=frontend /src/web/dist ./web/dist
# 编译缓存挂 BuildKit cache：GOCACHE 跨构建保留已编译包，
# 代码小改时 go build 从 ~25s 降到 ~5s（每次 import 变化才重编受影响包）
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/beacontower .

# ---------- 阶段 3：运行镜像 ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 beacon \
    && mkdir -p /app/data && chown beacon:beacon /app/data
USER beacon
WORKDIR /app
COPY --from=backend /out/beacontower /app/beacontower
VOLUME ["/app/data"]
EXPOSE 8080
HEALTHCHECK --interval=60s --timeout=3s --start-period=15s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/beacontower"]
