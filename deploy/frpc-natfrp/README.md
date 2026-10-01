# frpc-natfrp · 樱花穿透客户端容器

Sakura（SakuraFrp）官方 `natfrp/frpc` 二进制的容器封装，供 BeaconTower 的
「内网穿透平台托管」模块配套使用：面板负责建隧道/取配置，这个容器负责把
本机服务接到节点上。

- 镜像：`ghcr.io/yoahoug/beacontower-frpc-natfrp:latest`（另有 `:<版本>` 标签）
- 基底：官方镜像里的原始 frpc 二进制（静态编译）搬进 alpine，补 shell 与 CA 证书
- 版本：`0.51.0-sakura-14`（樱花分支，与面板 `NatfrpFrpcVersion` 一致）
- 架构：`linux/amd64`、`linux/arm64`

## 一键起

```sh
cd /data/appdata/frpc-natfrp            # 任意目录
curl -fsSLO https://raw.githubusercontent.com/Yoahoug/BeaconTower/main/deploy/frpc-natfrp/docker-compose.yml

# 面板 → 内网穿透 → 隧道 → 「配置」下载，把文件传上来（如 natfrp-ssh.conf）
FRPC_CONFIG_FILE=./natfrp-ssh.conf docker compose up -d
docker logs -f frpc-natfrp              # 看到 "start proxy success" 即通
```

或者直接用官方环境变量模式（不需要配置文件）：

```sh
docker run -d --name frpc-natfrp --restart unless-stopped --network host \
  -e NATFRP_TOKEN='访问密钥' -e NATFRP_TARGET='隧道ID1,隧道ID2' \
  ghcr.io/yoahoug/beacontower-frpc-natfrp:latest
```

> `NATFRP_TARGET` 是**逗号分隔的隧道 ID 列表**（如 `1234,6666`，隧道 ID 在面板隧道列表里），
> 不是节点域名——客户端拿到 ID 后自己去官方接口取配置，节点信息在配置里。
> 环境变量模式同样支持多节点（frpc v0.42.0-sakura-6+）。

## 配置格式为什么是 ini

面板取配置时会声明客户端版本，Sakura 按版本返回不同格式：樱花分支版本
（`0.51.0-sakura-N`）返回 INI（`sakura_mode = true`），上游 frp 版本（如
`0.59.0`）返回 TOML。面板固定声明樱花分支版本，与本镜像里的客户端一致 ——
**升级镜像版本时要同步改 `internal/frp/natfrp.go` 的 `NatfrpFrpcVersion` 并重建面板镜像**，
否则面板下载的配置格式会和新镜像对不上。

## 注意

- **必须 host 网络**：配置里的 `local_ip` 通常是 `localhost`/`127.0.0.1`（本机服务）。
  要用 bridge，得把配置里的 `local_ip` 改成宿主机地址或网关。
- 配置文件里有访问密钥，挂载建议只读（`:ro`），别提交进 git。
- 容器日志里出现「登录节点失败」先查网络（容器内能不能到节点域名）与密钥是否有效。
- 挂载的配置文件会被当作 Go template 渲染，可以用 `{{ .Envs.变量名 }}` 引用环境变量
  （官方行为），把密钥留在挂载文件之外时用得上。
