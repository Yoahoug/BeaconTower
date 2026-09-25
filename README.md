# BeaconTower · 信标塔

自托管服务器监控面板：亮色灵动「天空信标」前端，通过 SSH 直连被监控服务器采集指标（无 Agent），
Docker 单容器部署，公开页面不暴露 IP 等敏感信息。支持 RAPL 功耗监控（实时功率 / kWh / 电费）。

## 当前状态：v3.0 全栈落地（前端 v2.0 + Go 后端 M1–M6）

完整开发方案见 [doc/README.md](./doc/README.md)。

```sh
# 方式一：Docker 单容器（推荐）
cp .env.example .env   # 填入 BEACON_MASTER_KEY（openssl rand -hex 32）
docker compose up -d --build
# 打开 http://127.0.0.1:8080（经反代对外），首次访问 /admin 完成初始化

# 方式二：本地开发（前后端分离）
cd web && npm install && npm run dev   # http://localhost:5173（/api 代理到 8080）
BEACON_DATA_DIR=./data go run .        # http://127.0.0.1:8080
```

- **视觉**：浅色极光背景（马卡龙光斑漂移 + 信标光束）+ 白玻璃卡片 + 青蓝渐变品牌色；
  设计语言与动效清单见 [doc/06-前端设计规范.md](./doc/06-前端设计规范.md)
- **动效**：数字平滑补间、渐变仪表环、走势描画、指针辉光、脉冲状态点、spring 弹窗、
  页面转场——全部尊重 `prefers-reduced-motion`
- `/` 监控状态总览（KPI 玻璃条 + 全网吞吐/功耗趋势 + 节点玻璃卡网格 + 搜索/筛选，SSE 实时 + 10 秒保底轮询）
- `/server/:id` 节点详情（公开只读：1h/6h/24h/7d 分段 + CPU/内存磁盘/吞吐/功耗历史曲线，7d 默认需登录；总览卡片标题与“历史曲线”双入口进入）
- `/admin/servers|settings|security|audit` 管理面板：初始化向导 → 登录 → 节点管理（SSH 试连回读画像 / RAPL 能力探测 / 功耗校准）、采集与展示设置、安全与账号、审计日志
- 前端开发约束见 [doc/10-前端开发约束.md](./doc/10-前端开发约束.md)（合并门槛：a11y 自查 + 性能预算 + 动效预算）

> 采集到的指标均为被监控机的真实数据（经 SSH 只读命令采集）。功耗算法设计（RAPL / 电池校准 / kWh 积分）见 [doc/09-功耗监控设计.md](./doc/09-功耗监控设计.md)。

## 技术栈

- **后端**：Go 1.26 + Gin + SQLite（modernc.org/sqlite 纯 Go）+ golang.org/x/crypto/ssh
- **前端**：Vue 3 + Vite + Vue Router + Pinia + ECharts（按需引入），构建产物经 `go:embed` 打进单二进制
- **采集**：面板定时 SSH 连接各节点执行只读命令（读 `/proc`、`df` 等），被监控机零安装
- **安全**：SSH 凭据 AES-256-GCM 加密存储、Argon2id 密码哈希、首次访问初始化向导、公开 API 字段白名单
- **部署**：多阶段 Dockerfile 单镜像（~35MB）+ docker-compose，建议反代 HTTPS（Caddy 示例见 doc/07）
