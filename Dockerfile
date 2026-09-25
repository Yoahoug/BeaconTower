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
# 将阶段 1 产物拷入 web/dist，供 main.go 的 //go:embed 打进二进制
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
