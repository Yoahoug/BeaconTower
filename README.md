# BeaconTower · 信标塔

**自托管服务器监控面板** —— 无 Agent 采集、亮色玻璃拟态、带功耗计量、WireGuard 组网管理与内网穿透平台托管。
通过 SSH 直连被监控机采集真实指标，Docker 单容器部署，公开页永不暴露 IP 等敏感信息。

<p align="center">
  <img src="docs/img/overview.png" alt="BeaconTower 总览页：KPI 玻璃条 + 全网吞吐趋势 + 节点卡片网格" width="920">
</p>

<p align="center">
  <a href="#功能一览">功能</a> ·
  <a href="#wg-组网管理">WG 组网</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="#部署">部署</a> ·
  <a href="./doc/README.md">开发文档</a>
</p>

## 功能一览

- **零 Agent 采集**：面板定时 SSH 连接各节点执行只读命令（`/proc`、`df`、`sysfs`），被监控机不装任何东西；面板自身自动作为「本机」节点展示（本地进程采集，免 SSH）
- **系统画像自动回读**：添加节点只需 SSH 地址 + 账号，主机名 / 系统 / CPU / 内存 / 磁盘 / 虚拟化 / 公网 IP 归属自动探测
- **RAPL 功耗监控**（Intel 机型 / 含电池笔记本）：实时整机功率、CPU/内存分项、温度、频率；电池放电自动校准基础功耗，月度 kWh 与电费估算
- **实时总览**：KPI 玻璃条 + 全网吞吐趋势 + 节点卡片网格，SSE 推送 + 10 秒保底轮询；节点详情页 1h/6h/24h/7d 历史曲线
- **WG 组网管理**（v2.0 新增）：在面板里一键完成 WireGuard 星型组网——密钥生成、conf 渲染推送、防火墙放行、成员增删全部自动化，见[下文](#wg-组网管理)
- **内网穿透平台托管**（v2.1 新增）：接入 NATFRP（樱花）与 ChmlFrp 两个国内内网穿透平台——账号用量与余额、隧道在线状态与今日/累计流量、节点负载与倍率一览；支持新建/改参/启停/锁定/迁移隧道、下载 frpc 配置、ChmlFrp 子域名管理；OAuth 令牌自动续期，失效时面板内弹窗重新授权，见[下文](#内网穿透平台托管)
- **穿透状态（游客版）**：公开页多一个 `/tunnels`——今日流量消耗、隧道在线数与**在用**节点健康度（名称/分组/负载/在线时长）。账号画像、套餐余量与节点域名不在白名单内，公开页永不渲染（设计见 [doc/13 §12](./doc/13-内网穿透平台管理设计.md)）
- **管理面板**：初始化向导 → 登录 → 节点管理（试连回读画像、排序/隐藏/暂停、功耗校准）、采集展示设置、安全与账号（会话管理 / 改密踢会话）、审计日志
- **安全设计**：SSH 凭据 AES-256-GCM 加密存储、Argon2id 密码哈希、CSRF 双提交、登录限速封禁、TOFU 主机指纹（可选严格模式）、公开 API 字段白名单
- **视觉**：「天空信标」亮色玻璃拟态 —— 极光背景 + 白玻璃卡片 + 青蓝渐变，动效全部尊重 `prefers-reduced-motion`

## 截图

| 节点详情 · 资源/功耗一屏尽览 | 历史曲线 · 功耗与每日流量 |
| --- | --- |
| <img src="docs/img/node-detail.png" alt="节点详情页：CPU/内存/磁盘环形图、实时吞吐、RAPL 功耗条" width="440"> | <img src="docs/img/node-history.png" alt="历史曲线：网络吞吐、整机功耗、每日流量" width="440"> |

## WG 组网管理

面向「多台 VPS + 家庭 NAT」场景的 WireGuard 控制面：利用节点管理中已录入的 SSH 凭据，
把手工逐台 `wg genkey` / 手写 conf / 敲防火墙规则的过程，压缩成面板里的几次点击。

<p align="center">
  <img src="docs/img/wg-mesh.png" alt="WG 组网页：主备中心卡片 + 星型拓扑图 + 成员列表" width="920">
</p>

- **一键组网**：选中心 + 勾成员 → 面板生成密钥、渲染 conf、经 SSH 推送、自动放行 UDP、拉起接口、逐节点验证
- **热操作不断连**：hub 增删 peer 走 `wg set` + conf 追加；成员变更用 `wg syncconf`，存量隧道零中断
- **主备中心（A/B 轮换）**：现役 + 温备援随时一键切换；新机接管可**继承现役身份**（成员只改 Endpoint 一行）
- **凭证中心**：使用端（Mac/Win/iPhone）conf 在线预览、二维码扫码、打包下载（zip 内按平台命名）；每台设备一个 peer，PSK 可选
- **巡检**：每 5 分钟读 `wg dump` 判活、按日累计中心转发流量（月度额度提醒）、公钥漂移/配置外部改动告警
- **面板宿主可入网**：本机节点经宿主 SSH 接入组网（容器无 NET_ADMIN 时走宿主管理）

<p align="center">
  <img src="docs/img/wg-hub-detail.png" alt="中心节点凭证中心：端点概览、主备就绪度、使用端凭证（预览/QR/同步/下载）" width="680">
</p>

> 面板只写 WG 相关状态（conf / 防火墙 / sysctl），其余一律只读。设计细节见 [doc/12](./doc/12-WG组网模块设计.md)。
> 组网页截图来自合成数据的演示实例（IP 均为文档保留段 203.0.113.x / 10.66.66.x），非真实环境。

## 内网穿透平台托管

接入 NATFRP（樱花）与 ChmlFrp 两个国内内网穿透平台，两平台数据**归一化展示在同一份视图里**：
平台卡（配额/剩余流量/签到）→ 账号用量 → 流量历史曲线 → 隧道统一列表（跨平台新建/改参/启停/锁定/迁移、下载 frpc 配置）→ 在用节点负载；ChmlFrp 免费二级域名管理。OAuth 令牌自动续期，失效时面板内弹窗重新授权。

<p align="center">
  <img src="docs/img/frp-admin.png" alt="内网穿透管理页：双平台卡片、账号用量、流量历史、隧道统一列表与在用节点负载" width="920">
</p>

游客版公开页多一个 `/tunnels`——今日流量消耗、隧道在线数与**在用**节点健康度（名称/分组/负载/在线时长）。账号画像、套餐余量与节点域名不在白名单内，公开页永不渲染（设计见 [doc/13 §12](./doc/13-内网穿透平台管理设计.md)）。

<p align="center">
  <img src="docs/img/frp-public.png" alt="游客穿透状态页：今日流量、在线隧道、在用节点、实时连接数与双平台卡片" width="920">
</p>

> 截图来自合成数据的演示实例（隧道/节点名与流量均为演示值，节点域名统一为 `*.demo.example` 保留样式域），非真实环境。

## 快速开始

```sh
# Docker 单容器（推荐）
cp .env.example .env   # 填入 BEACON_MASTER_KEY（openssl rand -hex 32）
docker compose up -d --build
# 打开 http://127.0.0.1:8080，首次访问 /admin 完成管理员初始化

# 本地开发（前后端分离）
cd web && npm install && npm run dev   # http://localhost:5173（/api 代理到 8080）
BEACON_DATA_DIR=./data go run .        # http://127.0.0.1:8080
```

## 技术栈

| 层 | 选型 |
|---|---|
| 后端 | Go 1.26 + Gin + SQLite（modernc.org/sqlite 纯 Go，无 CGO） |
| 前端 | Vue 3 + Vite + Pinia + ECharts（按需引入），构建产物 `go:embed` 进单二进制 |
| 采集 | SSH（golang.org/x/crypto）执行只读脚本；本机节点走本地进程 |
| 组网 | WireGuard 内核态 + wg-quick，面板经 SSH 全程托管 |
| 部署 | 多阶段 Dockerfile 单镜像（~35MB）+ docker-compose |

## 部署

镜像由 GitHub Actions 自动构建并发布到 GHCR（push 到 `main` 即触发，`linux/amd64`）：

```sh
# 服务器上
mkdir -p /data/appdata/beacontower && cd /data/appdata/beacontower
cat > docker-compose.yml <<'EOF'
services:
  beacontower:
    image: ghcr.io/yoahoug/beacontower:latest
    container_name: beacontower
    restart: unless-stopped
    ports:
      - "127.0.0.1:8091:8080"   # 只绑本机回环，走反代对外
    environment:
      - BEACON_MASTER_KEY=<openssl rand -hex 32>
      - TZ=Asia/Shanghai
    volumes:
      - ./data:/app/data
EOF
echo "BEACON_MASTER_KEY=$(openssl rand -hex 32)" >> .env
docker compose up -d
```

建议前置反向代理提供 HTTPS（Caddy 示例见 [doc/07-Docker部署方案.md](./doc/07-Docker部署方案.md)）。

## 隐私与安全

- 公开页与公开 API 走**字段白名单**：不输出公网 IP、内网地址、主机名等敏感字段，隐藏节点对外统一「不存在」
- SSH 密码/私钥、WG 私钥/PSK 全部 AES-256-GCM 加密落库，主密钥来自环境变量或 `data/master.key`（0600）
- 登录限速 + 连续失败递增封禁；审计日志中来源 IP 只存哈希
- 建议部署在反向代理（HTTPS）之后，管理端仅自己访问

## 文档

完整开发方案（需求 / 架构 / 数据库 / API / 安全 / 前端规范 / 部署 / 功耗算法 / WG 组网）见 [doc/README.md](./doc/README.md)：

- [01-需求分析](./doc/01-需求分析.md) · [02-总体架构与技术选型](./doc/02-总体架构与技术选型.md) · [03-数据库与数据模型](./doc/03-数据库与数据模型.md)
- [04-API接口设计](./doc/04-API接口设计.md) · [05-安全设计](./doc/05-安全设计.md) · [06-前端设计规范](./doc/06-前端设计规范.md)
- [07-Docker部署方案](./doc/07-Docker部署方案.md) · [09-功耗监控设计](./doc/09-功耗监控设计.md) · [11-服务器部署SOP](./doc/11-服务器部署SOP.md)
- [12-WG组网模块设计](./doc/12-WG组网模块设计.md)

> 采集到的指标均为被监控机真实数据（SSH 只读命令），面板不写目标机任何状态（WG 组网除外，且仅限 WG 配置）。

## 友情链接

- [LINUX DO](https://linux.do/) —— 中文技术社区（AI / 开发 / 自托管），Where possible begins.
