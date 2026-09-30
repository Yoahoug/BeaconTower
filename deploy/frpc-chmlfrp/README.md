# frpc-chmlfrp · ChmlFrp 穿透客户端容器

ChmlFrp 面板（panel.chmlfrp.net）下发的 frpc 配置的配套客户端容器。
面板建隧道/取配置，这个容器把本机服务接到节点上。

- 镜像：`ghcr.io/yoahoug/beacontower-frpc-chmlfrp:latest`（另有 `:<版本>` 标签）
- 客户端：上游 [fatedier/frp](https://github.com/fatedier/frp) release 二进制 `0.61.2`
- 架构：`linux/amd64`、`linux/arm64`

## 为什么用上游 frp 而不是 ChmlFrp 的魔改分支

ChmlFrp 官方仓库 `TechCat-Team/ChmlFrp-Frp` 是 frp **0.51.2** 的旧魔改（quic-go
v0.36 卡在 Go 1.20，新工具链编不过，且久未跟进上游安全修复）。实测上游 frp
0.61.2 的 frpc 配 ChmlFrp 面板下发的 ini 可以正常登录、起隧道，而且它与 ops
服务器上长期在跑的容器二进制 sha256 完全一致（`8b82594c…`），所以直接取官方
release，更好维护。

## 一键起

```sh
mkdir -p /data/appdata/frpc-chmlfrp && cd /data/appdata/frpc-chmlfrp
curl -fsSLO https://raw.githubusercontent.com/Yoahoug/BeaconTower/main/deploy/frpc-chmlfrp/docker-compose.yml

# 面板 → 内网穿透 → 隧道 → 「配置」下载，把文件传上来（如 chmlfrp-ssh.conf）
FRPC_CONFIG_FILE=./chmlfrp-ssh.conf docker compose up -d
docker logs -f frpc-chmlfrp               # 看到 "start proxy success" 即通
```

不用 compose 也可以：

```sh
docker run -d --name frpc-chmlfrp --restart unless-stopped --network host \
  -v "$PWD/chmlfrp-ssh.conf:/etc/frp/frpc.ini:ro" \
  ghcr.io/yoahoug/beacontower-frpc-chmlfrp:latest
```

## 注意

- **必须 host 网络**：配置里的 `local_ip` 通常是 `localhost`/`127.0.0.1`（本机服务）。
  要用 bridge，得把配置里的 `local_ip` 改成宿主机地址或网关。
- 配置文件里有平台 token，挂载建议只读（`:ro`），别提交进 git。
- 升级版本：手动触发 `FRP client images (manual)` workflow，把 frp 版本作为输入；
  面板下载的 ini 与新版本仍兼容（上游 frp 一直保留 ini 解析）。
