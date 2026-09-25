# BeaconTower · 信标塔

自托管服务器监控面板：亮色卡片式前端，通过 SSH 直连被监控服务器采集指标（无 Agent），Docker 单容器部署，公开页面不暴露 IP 等敏感信息。支持 RAPL 功耗监控（实时功率 / kWh / 电费）。

## 当前状态：前端原型（含完整管理面板 + 功耗展示）

完整开发方案见 [doc/README.md](./doc/README.md)。前端原型（液态玻璃风格卡片墙 + 环形仪表 + 功耗监控，模拟数据）已可运行：

```sh
cd web
npm install
npm run dev        # http://localhost:5173
```

- `/` 监控状态总览（卡片墙 + 汇总统计 + 全网功耗走势，10 秒静默刷新）
- `/admin` 管理面板：初始化向导 → 登录 → 节点管理（SSH 试连回读画像 / RAPL 能力探测 / 功耗校准）、采集与展示设置、安全与账号、审计日志
- 原型阶段管理数据保存在浏览器 localStorage（`web/src/admin/store.js` 对齐后端 API 语义，M1 后端就绪后直接替换该文件）

> 所有数据均为前端模拟数据，不含真实服务器信息。功耗算法设计（RAPL / 电池校准 / kWh 积分）见 [doc/09-功耗监控设计.md](./doc/09-功耗监控设计.md)。

## 技术栈（规划）

- **后端**：Go + Gin + SQLite（无独立数据库服务）+ golang.org/x/crypto/ssh
- **前端**：Vue 3 + Vite + Vue Router，构建产物经 `go:embed` 打进单二进制
- **采集**：面板定时 SSH 连接各节点执行只读命令（读 `/proc`、`df` 等），被监控机零安装
- **安全**：SSH 凭据 AES-256-GCM 加密存储、Argon2id 密码哈希、首次访问初始化向导、公开 API 字段白名单
- **部署**：多阶段 Dockerfile 单镜像 + docker-compose，建议反代 HTTPS
