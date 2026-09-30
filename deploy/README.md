# deploy/ · 部署资产

面板本体之外，跟「装到服务器上」有关的东西都放这里。

| 目录 | 内容 | 什么时候用 |
| --- | --- | --- |
| [`frpc-natfrp/`](./frpc-natfrp/) | NATFRP（樱花）frpc 客户端容器 | 面板里绑了 NATFRP 账号，要把本机服务经樱花节点暴露 |
| [`frpc-chmlfrp/`](./frpc-chmlfrp/) | ChmlFrp frpc 客户端容器 | 面板里绑了 ChmlFrp 账号，同上 |

<div align="center">

**客户端镜像不跟随 main 推送构建** —— 它们是第三方客户端的封装，版本由上游决定，
在 Actions 页手动触发 `FRP client images (manual)` 才会构建（可传版本号输入）。

</div>

## 它们和面板的关系

面板（`ghcr.io/yoahoug/beacontower`）管的是**本机**的内网穿透：建隧道、改参数、
看流量，但真正把流量从节点送到本机服务的，是跑在这台机器上的 frpc 进程。
干净服务器上的顺序是：

```sh
# 1. 起面板（见仓库 README「快速开始」）
# 2. 面板里初始化 → 绑定 NATFRP / ChmlFrp 账号 → 建隧道
# 3. 隧道列表点「配置」下载 ini，落到 frpc-* 目录，docker compose up -d
```

两个客户端镜像都默认 `network_mode: host`，因为隧道配置里的 `local_ip` 通常是
`localhost`/`127.0.0.1`——指宿主机上的服务，而 bridge 网络里的 localhost 是容器自己。

## 配置格式的版本耦合（升级前必看）

| 平台 | 面板请求的客户端版本 | 配置格式 | 对应镜像 |
| --- | --- | --- | --- |
| NATFRP | `NatfrpFrpcVersion = 0.51.0-sakura-14` | INI（`sakura_mode = true`） | `beacontower-frpc-natfrp` |
| ChmlFrp | 不适用（平台固定返回 INI） | INI | `beacontower-frpc-chmlfrp`（上游 frp `0.61.2`） |

NATFRP 的配置格式跟着「声明的客户端版本」走：樱花分支版本给 INI，上游 frp 版本
（如 `0.59.0`）给 TOML。**升级 `beacontower-frpc-natfrp` 的版本时，要同步改
`internal/frp/natfrp.go` 里的 `NatfrpFrpcVersion` 并重建面板镜像**，否则下载的配置
和新镜像里的客户端对不上。ChmlFrp 没有这个问题：平台只发 INI，上游 frp 一直兼容。
