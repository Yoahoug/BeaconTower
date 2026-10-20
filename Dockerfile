# ---------- 阶段 1a：构建 toy 前端（/toy/ 主题） ----------
FROM node:22-alpine AS frontend-toy
WORKDIR /src/web-toy
COPY web-toy/package.json web-toy/package-lock.json ./
# npm 缓存挂 BuildKit cache：package-lock 不变时 npm ci 只从本地缓存解包，不走网络
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-fund --no-audit --prefer-offline
COPY web-toy/ ./
RUN npm run build          # 产出 /src/web-toy/dist（vue-tsc && vite build）

# ---------- 阶段 1b：构建 classic 前端（Sky Glass，默认主题，挂根路径） ----------
FROM node:22-alpine AS frontend-classic
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-fund --no-audit --prefer-offline
COPY web/ ./
RUN npm run build          # 产出 /src/web/dist

# ---------- 阶段 2：构建后端（纯 Go，无 CGO） ----------
FROM golang:1.26-alpine AS backend
# GOPROXY 可构建时覆盖：默认源在部分出口下会 TLS 中断（bad record MAC），
# 此时用 --build-arg GOPROXY=https://goproxy.cn,direct 换源重试
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
# 模块缓存挂 BuildKit cache：go.sum 不变时秒级完成（原每次全量下载 ~8s）
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
# 将两套前端产物拷入对应 dist 目录，供 main.go 的 //go:embed 打进二进制
# （与 frontend-toy / frontend-classic 两个阶段并行、独立缓存层）
COPY --from=frontend-toy /src/web-toy/dist ./web-toy/dist
COPY --from=frontend-classic /src/web/dist ./web/dist
# 编译缓存挂 BuildKit cache：GOCACHE 跨构建保留已编译包，
# 代码小改时 go build 从 ~25s 降到 ~5s（每次 import 变化才重编受影响包）
# go build 也挂 /go/pkg/mod：download 与 build 分层后，改代码时 build 层
# 需要的模块 zip 可能不在缓存里，得重新拉（曾因此撞上出口抖动构建失败）
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
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
