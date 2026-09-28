# 服务器部署 SOP（ops）

> 目标机：`10.66.66.66`（server-ops skill）。镜像由 GitHub Actions 构建发布到 GHCR，
> 服务器只做 pull + compose up，构建不占服务器资源。

## 首次部署（已完成 2026-09-26）

```sh
# 1. 建目录 + 编排 + 密钥
mkdir -p /data/appdata/beacontower/data
# docker-compose.yml 内容见仓库 docker-compose.deploy.yml（见下）
KEY=$(openssl rand -hex 32)
echo "BEACON_MASTER_KEY=$KEY" > /data/appdata/beacontower/.env   # chmod 600

# 2. 修数据目录属主（容器内 uid 10001 非 root；root 属主目录 SQLite 打不开 → error 14）
chown -R 10001:10001 /data/appdata/beacontower/data

# 3. 拉镜像并启动
cd /data/appdata/beacontower && docker compose pull && docker compose up -d

# 4. 验证
docker ps --filter name=beacontower --format "{{.Status}}"   # healthy
curl -s http://127.0.0.1:8091/healthz                         # OK
curl -s http://127.0.0.1:8091/api/v1/public/servers           # 本机节点 online
```

## 日常更新（GitHub 构建 → 服务器拉取）

```sh
# 本地：push 到 main 即自动构建镜像（~2 分钟）
git push origin main
gh run watch --repo Yoahoug/BeaconTower   # 可选：盯构建

# 服务器：拉新镜像 + 滚动重建
cd /data/appdata/beacontower
docker compose pull && docker compose up -d
docker image prune -f                      # 清理悬空旧镜像
```

## docker-compose.yml（/data/appdata/beacontower/）

```yaml
services:
  beacontower:
    image: ghcr.io/yoahoug/beacontower:latest
    container_name: beacontower
    restart: unless-stopped
    user: root                      # RAPL energy_uj 宿主上仅 root 可读(0400)，非 root 容器读不到
    ports:
      - "10.66.66.66:8091:8080"   # 绑组网地址（组网内设备可直接访问）；走公网反代则改 127.0.0.1
    environment:
      - BEACON_MASTER_KEY=${BEACON_MASTER_KEY}   # .env 提供，32 字节 hex
      - TZ=Asia/Shanghai
    volumes:
      - ./data:/app/data          # SQLite 库 + master.key
      # 宿主 /proc 只读挂载：本机节点网络/连接数/进程数与 CPU/内存同口径。注意 /proc/net
      # 是 netns 维度的，采集侧经宿主 PID 1（/host/proc/1/net）读取宿主网卡计数
      - /proc:/host/proc:ro
      # 宿主 /sys 只读挂载：容器内 /sys 只见容器自身网卡（netns 维度），挂载后才能按
      # 「物理网卡」口径统计流量（排除 br0/veth/wg0/TUN 与物理网卡重复计数）。
      # 须挂整个 /sys：class/net/<if>/device 是指向 /sys/devices/... 的符号链接，
      # 只挂子目录会因链接悬空而判不出物理网卡。未挂载时退化为「流量最大的单接口」。
      - /sys:/host/sys:ro
      # Docker 默认 MaskedPaths 掩掉 /sys/devices/virtual/powercap（class 符号链接失效），
      # 只读挂到 /powercap-ro 兜底，采集脚本自动探测两种布局：
      - /sys/devices/virtual/powercap:/powercap-ro:ro
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/healthz"]
      interval: 30s
      timeout: 3s
      retries: 3
```

## 排查记录

| 症状 | 原因 | 处置 |
|---|---|---|
| `打开数据库失败: unable to open database file (14)` 循环重启 | 宿主 `data/` 为 root 属主，容器内 beacon(uid 10001) 不可写 | `chown -R 10001:10001 data/`；新镜像已在构建期 `mkdir + chown /app/data`，匿名卷首挂载继承镜像属主，全新部署不再需要手工 chown |
| 容器内读不到 RAPL（`/sys/class/powercap/intel-rapl:0` 符号链接失效） | Docker 默认 MaskedPaths 掩了 `/sys/devices/virtual/powercap` | compose 只读挂载宿主该目录到 `/powercap-ro`；采集脚本（v6503c8a+）自动探测标准路径与兜底路径两种布局 |
| `/powercap-ro` 能看到目录但 `cat energy_uj` Permission denied | 宿主 energy_uj 权限 0400 root，非 root 容器不可读 | compose 加 `user: root`（i5-6300HQ 实测 total_w/cpu_w/temp/freq 全部出数） |
| 本机节点网络速率/进程数明显偏小（与宿主对不上） | 容器 netns/pidns 隔离：`/proc/net/dev`、`/proc` 目录只见面板容器自身 | compose 只读挂宿主 `/proc` 到 `/host/proc`（v2026-09-27+ 采集自动优先读取），未挂载时回落容器视图（口径降级）。**线上 compose 若来自更早版本需手工补这一行并 `docker compose up -d` 重建容器** |
| 本机上下行只有 1~2 KB/s，比宿主真实流量小几个数量级 | `/proc/net` 是 **netns 维度**的（等价 `/proc/self/net`，self＝读取进程）：挂了宿主 `/proc` 后读 `/host/proc/net/dev` 拿到的仍是**面板容器自己**的网卡计数（线上实测 989 KB，宿主 `enp3s0f1` 已 157 GB） | 采集改经宿主 PID 1 读取 `/host/proc/1/net/dev`，tcp/udp 连接数同目录（v2026-09-28+）。升级后本机累计收发计数器与宿主 `/proc/net/dev` 逐字节对齐 |
| 本机/节点流量偏大或翻倍（桥接、隧道主机） | 原口径把 `/proc/net/dev` 全部非 lo 接口相加，而桥/隧道/veth 的字节数与物理网卡是同一份流量：`br0` 与成员 `enp3s0f1`、`eth0` 与 `wg0`/TUN 各计一次 | 只统计**物理网卡**（sysfs `class/net/<if>/device` 存在＝真实 PCI/USB 设备）；compose 加 `- /sys:/host/sys:ro`（须整目录，否则 device 符号链接悬空）；判据不可用时退化为「累计字节最多的单接口」。远端节点同口径（采集脚本内同一判据） |
| 升级到新口径后，「今日流量」出现一次性巨幅跳变（如本机今日 +157 GB） | 日/小时聚合是「窗口首尾累计计数器差」，累计计数器口径切换（容器视图→宿主网卡）当天会形成一次巨幅跳变 | 仅影响升级当天：升级后把当天**旧口径样本**的累计字段置空，下一次聚合（≤10min）即按新基线重算。`TODAY=$(date -d "today 00:00" +%s)`；`NEW=$(sqlite3 $DB "SELECT min(ts) FROM metric_sample WHERE server_id=1 AND ts>=$TODAY AND net_in_total>1000000000")`；`sqlite3 $DB "UPDATE metric_sample SET net_in_total=NULL,net_out_total=NULL WHERE server_id=1 AND ts>=$TODAY AND ts<$NEW"`（先 `.backup` 热备份）。历史日行不受影响，次日自愈 |
| 面板 CPU 使用率与网络速率**全部节点恒为 0**（内存/磁盘/负载正常） | `applySample` 漏写差分基线（`prevState` 的 ts/cpuTotal/netRx 从不回写），两级差分永不成立 | 已修（`diffMetrics` 统一算差值并写回基线，含回归单测）；升级到含该修复的镜像即可自愈，无需改数据 |
| 任务永远「执行中」，所有组网按钮返回 1004 | 执行 goroutine 意外退出/早退漏收尾，`wg_task.status` 卡在 running | 启动清理 + **运行时看门狗**（5min ticker 回收超过 30min 的 running 任务）；早退路径已补 `FinishWGTask` |

## 相关入口

- 面板（组网）：http://10.66.66.66:8091（绑定组网地址，组网内设备可直接访问）
- GHCR 镜像：`ghcr.io/yoahoug/beacontower:latest`（另含 `${sha}` tag 可回滚）
- 构建日志：GitHub 仓库 → Actions → "Docker image (GHCR)"

## WG 组网模块（doc/12）运维要点

- **现状**：星型组网，hub 虚拟 IP `10.66.66.2` 浮动于 wg1(47.109.156.165:51820) 与
  wg2(47.108.59.19:51830) 之间；ops(10.66.66.66) 家庭 NAT 拨出；面板「WG 组网」页
  可导入纳管、一键切换、设备凭证二维码、额度巡检。
- **切换前检查**：目标 hub 的云安全组已放行其 UDP 端口（阿里云控制台，无法自动化）；
  切换后按任务摘要给设备（Mac/iPhone/Win）换 A/B 凭证（面板重新扫码）。
- **conf 备份**：面板每次改写前自动 `wg0.conf.bak.<时间戳>`；手工恢复用
  `wg-quick down wg0 && cp wg0.conf.bak.<ts> wg0.conf && wg-quick up wg0`。
- **wg2 遗留问题（2026-09-27 记录）**：与 wg1 同占 10.66.66.2/24、conf 无 PostUp
  （FORWARD 规则手工加、重启丢失）、peer 全部 4 天零握手。处理：面板导入纳管为备胎后
  走「校正备援 hub」重写 conf（自动补 PostUp 幂等放行），遗留 `.bak`/残网人工归档。
- **巡检告警语义**：`接口公钥与面板记录不符` = 有人绕过面板手工改了 hub 配置；
  `handshake 超 3 分钟` = 成员离线（keepalive 25s 下正常活体 ≤2min 必有握手）。
- **本机节点**：v1 不支持经 SSH 管理本机 WG（面板容器无 NET_ADMIN）；ops 的 wg0
  仍按本文档手工维护，或在后续版本引入宿主凭据路径。
