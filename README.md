# BeaconTower · 信标塔

**自托管服务器监控面板** —— 无 Agent、亮色灵动、带功耗计量。通过 SSH 直连被监控机采集真实指标，
Docker 单容器部署，公开页永不暴露 IP 等敏感信息。

<p align="center">
  <a href="#快速开始">快速开始</a> ·
  <a href="#功能一览">功能</a> ·
  <a href="#技术栈">技术栈</a> ·
  <a href="#部署">部署</a> ·
  <a href="./doc/README.md">开发文档</a>
</p>

## 特性一览

- **零 Agent 采集**：面板定时 SSH 连接各节点执行只读命令（`/proc`、`df`、`sysfs`），被监控机不装任何东西；面板自身自动作为「本机」节点展示（本地进程采集，免 SSH）
- **系统画像自动回读**：添加节点只需 SSH 地址 + 账号，主机名 / 系统 / CPU / 内存 / 磁盘 / 虚拟化 / 公网 IP 归属自动探测
- **RAPL 功耗监控**（Intel 机型 / 含电池笔记本）：实时整机功率、CPU/内存分项、温度、频率；电池放电自动校准基础功耗，月度 kWh 与电费估算
- **实时总览**：KPI 玻璃条 + 全网吞吐趋势 + 节点卡片网格，SSE 推送 + 10 秒保底轮询；节点详情页 1h/6h/24h/7d 历史曲线
- **管理面板**：初始化向导 → 登录 → 节点管理（试连回读画像、排序/隐藏/暂停、功耗校准）、采集展示设置、安全与账号（会话管理 / 改密踢会话）、审计日志
- **安全设计**：SSH 凭据 AES-256-GCM 加密存储、Argon2id 密码哈希、CSRF 双提交、登录限速封禁、TOFU 主机指纹（可选严格模式）、公开 API 字段白名单
- **视觉**：「天空信标」亮色玻璃拟态 —— 极光背景 + 白玻璃卡片 + 青蓝渐变，动效全部尊重 `prefers-reduced-motion`

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

## 文档

完整开发方案（需求 / 架构 / 数据库 / API / 安全 / 前端规范 / 部署 / 功耗算法）见 [doc/README.md](./doc/README.md)：

- [01-需求分析](./doc/01-需求分析.md) · [02-总体架构与技术选型](./doc/02-总体架构与技术选型.md) · [03-数据库与数据模型](./doc/03-数据库与数据模型.md)
- [04-API接口设计](./doc/04-API接口设计.md) · [05-安全设计](./doc/05-安全设计.md) · [06-前端设计规范](./doc/06-前端设计规范.md)
- [07-Docker部署方案](./doc/07-Docker部署方案.md) · [09-功耗监控设计](./doc/09-功耗监控设计.md) · [10-前端开发约束](./doc/10-前端开发约束.md)

> 采集到的指标均为被监控机真实数据（SSH 只读命令），面板不写目标机任何状态。
