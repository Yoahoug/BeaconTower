# ChmlFrp frpc 客户端（BeaconTower 内网穿透模块配套）
#
# ChmlFrp 的客户端不需要它的魔改分支（TechCat-Team/ChmlFrp-Frp 停在 frp 0.51.2、
# 卡在 Go 1.20），上游 fatedier/frp 的 release 二进制就能登录它的节点、跑它的
# INI 配置 —— 0.61.2 与 ops 长期在跑的容器二进制 sha256 完全一致。
ARG FRPC_VERSION=0.61.2

FROM alpine:3.20 AS fetch
ARG FRPC_VERSION
ARG TARGETARCH
RUN apk add --no-cache ca-certificates wget \
 && case "$TARGETARCH" in \
      amd64) frp_arch=amd64 ;; \
      arm64) frp_arch=arm64 ;; \
      *) echo "不支持的 TARGETARCH: $TARGETARCH（本镜像只出 amd64 / arm64）" >&2; exit 1 ;; \
    esac \
 && wget -qO /tmp/frp.tar.gz "https://github.com/fatedier/frp/releases/download/v${FRPC_VERSION}/frp_${FRPC_VERSION}_linux_${frp_arch}.tar.gz" \
 && tar -xzf /tmp/frp.tar.gz -C /tmp \
 && install -D -m 0755 "/tmp/frp_${FRPC_VERSION}_linux_${frp_arch}/frpc" /out/frpc \
 && /out/frpc -v

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=fetch /out/frpc /usr/local/bin/frpc
COPY entrypoint.sh /usr/local/bin/entrypoint-frpc.sh
RUN chmod 0755 /usr/local/bin/frpc /usr/local/bin/entrypoint-frpc.sh
LABEL org.opencontainers.image.title="beacontower-frpc-chmlfrp" \
      org.opencontainers.image.description="ChmlFrp frpc client (upstream frp ${FRPC_VERSION}) for BeaconTower" \
      org.opencontainers.image.source="https://github.com/Yoahoug/BeaconTower"
ENTRYPOINT ["/usr/local/bin/entrypoint-frpc.sh"]
