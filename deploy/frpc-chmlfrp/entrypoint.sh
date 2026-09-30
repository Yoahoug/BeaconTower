#!/bin/sh
# 配置来自面板「内网穿透 → 隧道 → 配置」的下载（ini 文本），默认挂到
# /etc/frp/frpc.ini，可用 FRPC_CONFIG 覆盖路径。上游 frpc 没有环境变量
# 模式，必须给配置文件。
set -eu

cfg="${FRPC_CONFIG:-/etc/frp/frpc.ini}"

if [ ! -f "$cfg" ]; then
  cat >&2 <<USAGE
beacontower-frpc-chmlfrp: 找不到配置文件 $cfg
  把面板「内网穿透 → 隧道 → 配置」下载的 ini 挂载进来，例如:
    docker run -d --name frpc-chmlfrp --restart unless-stopped --network host \\
      -v "\$PWD/chmlfrp-xxx.conf:/etc/frp/frpc.ini:ro" \\
      ghcr.io/yoahoug/beacontower-frpc-chmlfrp:latest
提示：挂载的源文件不存在时 Docker 会创建同名目录，看到这行先检查路径。
USAGE
  exit 1
fi

exec /usr/local/bin/frpc -c "$cfg" "$@"
