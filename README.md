# BeaconTower · 信标塔

自托管服务器监控面板：亮色灵动「天空信标」前端，通过 SSH 直连被监控服务器采集指标（无 Agent），
Docker 单容器部署，公开页面不暴露 IP 等敏感信息。支持 RAPL 功耗监控（实时功率 / kWh / 电费）。

## 当前状态：前端 v2.0「天空信标」（含完整管理面板 + 功耗展示）

完整开发方案见 [doc/README.md](./doc/README.md)。前端（Vue 3 + Pinia + ECharts，模拟数据）已可运行：

```sh
cd web
npm install
npm run dev        # http://localhost:5173
```

- **视觉**：浅色极光背景（马卡龙光斑漂移 + 信标光束）+ 白玻璃卡片 + 青蓝渐变品牌色；
  设计语言与动效清单见 [doc/06-前端设计规范.md](./doc/06-前端设计规范.md)
- **动效**：数字平滑补间、渐变仪表环、走势描画、指针辉光、脉冲状态点、spring 弹窗、
  页面转场——全部尊重 `prefers-reduced-motion`
- `/` 监控状态总览（KPI 玻璃条 + 全网吞吐/功耗趋势 + 节点玻璃卡网格 + 搜索/筛选，10 秒静默刷新）
- `/admin/servers|settings|security|audit` 管理面板：初始化向导 → 登录 → 节点管理（SSH 试连回读画像 / RAPL 能力探测 / 功耗校准）、采集与展示设置、安全与账号、审计日志
- 原型阶段管理数据保存在浏览器 localStorage（`web/src/api/admin.js` 门面对齐后端 API 语义，M1 后端就绪后替换方法体即可，视图零改动）
- 前端开发约束见 [doc/10-前端开发约束.md](./doc/10-前端开发约束.md)（合并门槛：a11y 自查 + 性能预算 + 动效预算）

> 所有数据均为前端模拟数据，不含真实服务器信息。功耗算法设计（RAPL / 电池校准 / kWh 积分）见 [doc/09-功耗监控设计.md](./doc/09-功耗监控设计.md)。

## 技术栈（规划）

- **后端**：Go + Gin + SQLite（无独立数据库服务）+ golang.org/x/crypto/ssh
- **前端**：Vue 3 + Vite + Vue Router + Pinia + ECharts（按需引入），构建产物经 `go:embed` 打进单二进制
- **采集**：面板定时 SSH 连接各节点执行只读命令（读 `/proc`、`df` 等），被监控机零安装
- **安全**：SSH 凭据 AES-256-GCM 加密存储、Argon2id 密码哈希、首次访问初始化向导、公开 API 字段白名单
- **部署**：多阶段 Dockerfile 单镜像 + docker-compose，建议反代 HTTPS
