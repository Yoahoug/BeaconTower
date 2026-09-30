#!/bin/sh
# 配置来源二选一（配置文件优先）：
#   1) 配置文件：默认 /etc/frp/frpc.ini（可用 FRPC_CONFIG 改路径），
#      内容来自面板「内网穿透 → 隧道 → 配置」的下载；
#   2) 环境变量：NATFRP_TOKEN（访问密钥）+ NATFRP_TARGET（隧道 ID 列表，如 1234,6666）。
# 默认带 -n（不检查更新）：镜像已 pin 版本，容器里查更新没有意义。
set -eu

cfg="${FRPC_CONFIG:-/etc/frp/frpc.ini}"

if [ -f "$cfg" ]; then
  exec /usr/local/bin/frpc -c "$cfg" -n "$@"
fi

if [ -n "${NATFRP_TOKEN:-}" ] && [ -n "${NATFRP_TARGET:-}" ]; then
  exec /usr/local/bin/frpc -n "$@"
fi

cat >&2 <<'USAGE'
beacontower-frpc-natfrp: 没有可用的配置
  1) 配置文件模式 —— 把面板「内网穿透 → 隧道 → 配置」下载的文件挂载到
     /etc/frp/frpc.ini（或用 FRPC_CONFIG 指定路径）
  2) 环境变量模式 —— 设置 NATFRP_TOKEN（访问密钥）与 NATFRP_TARGET（隧道 ID 列表，如 1234,6666）
提示：挂载的源文件不存在时 Docker 会创建同名目录，先检查路径。
USAGE
exit 1
