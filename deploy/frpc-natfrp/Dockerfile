# 樱花 frpc 客户端（BeaconTower 内网穿透模块配套）
#
# 官方 natfrp/frpc 是 scratch 镜像：只有 /frpc 一个静态二进制，没有 shell，
# 没法写入口脚本、也没有 CA 证书。这里把那个二进制原样搬进 alpine，补上
# shell + ca-certificates + 入口脚本，注入方式保持两种：
#   1. 配置文件模式：挂载面板下载的 sakura INI（默认 /etc/frp/frpc.ini）
#   2. 官方环境变量模式：NATFRP_TOKEN（访问密钥）+ NATFRP_TARGET（节点域名）
ARG NATFRP_VERSION=0.51.0-sakura-14
FROM natfrp/frpc:${NATFRP_VERSION} AS upstream

FROM alpine:3.20
ARG NATFRP_VERSION
RUN apk add --no-cache ca-certificates
COPY --from=upstream /frpc /usr/local/bin/frpc
COPY entrypoint.sh /usr/local/bin/entrypoint-frpc.sh
RUN chmod 0755 /usr/local/bin/frpc /usr/local/bin/entrypoint-frpc.sh
LABEL org.opencontainers.image.title="beacontower-frpc-natfrp" \
      org.opencontainers.image.description="SakuraFrp frpc ${NATFRP_VERSION} client for BeaconTower" \
      org.opencontainers.image.source="https://github.com/Yoahoug/BeaconTower"
ENTRYPOINT ["/usr/local/bin/entrypoint-frpc.sh"]
