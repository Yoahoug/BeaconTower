# 04 · API 接口设计

- 前缀：`/api/v1`；除公开接口外均需管理员会话；
- 响应统一包装：`{"code": 0, "msg": "ok", "data": ...}`；错误码见文末；
- 静态前端由后端 `go:embed` 直接托管（SPA fallback 到 `index.html`）；
- **凭据不回显原则**（借鉴 Nezha 对敏感字段的处理）：所有读取接口永不返回 SSH 密码/私钥/口令，编辑时留空 = 保留原值。

## 1. 公开接口（访客，无鉴权）

### 1.1 快照

`GET /api/v1/public/summary`

```json
{ "code": 0, "data": { "total": 8, "online": 7, "offline": 1, "up_bps": 3210000, "down_bps": 11840000 } }
```

### 1.2 节点列表（含最新指标）

`GET /api/v1/public/servers`

> **白名单序列化**：下面就是这个接口能出现的全部字段，不存在其他字段。
> IP、主机名、SSH 端口/用户名、私有备注、失败原因等在 SQL 层就不会被查出来。
> `region` 由后端按节点公网出口 IP 自动定位写入（`region_source: "auto"`），管理员覆盖后为 `"manual"`；只回地理文案，永不回 IP。

```json
{
  "code": 0,
  "data": [{
    "id": 1,
    "name": "香港 · 轻量 01",
    "region": "香港",
    "region_source": "auto",
    "tags": ["主力", "Web"],
    "note_public": "",
    "status": "online",
    "profile": { "os": "Ubuntu", "os_version": "24.04", "arch": "x86_64",
                 "cpu_cores": 4, "mem_total": 4294967296, "disk_total": 64424509440, "virt": "KVM" },
    "metrics": { "ts": 1727067600, "cpu_pct": 23.1, "mem_used": 2254857830,
                 "swap_used": 0, "disk_used": 19756849500,
                 "net_in_bps": 1842000, "net_out_bps": 128400,
                 "tcp_conns": 214, "udp_conns": 37,
                 "load1": 0.42, "load5": 0.38, "load15": 0.35,
                 "uptime_s": 8294400, "processes": 168 },
    "power": { "total_w": 8.2, "cpu_w": 2.6, "dram_w": 0.5, "temp_c": 46.5,
               "freq_mhz": 1700, "power_source": "ac",
               "today_kwh": 0.014, "month_kwh": 6.062, "est_cost_month": 3.64 }
  }]
}
```

> `power` 字段说明见 [09-功耗监控设计](./09-功耗监控设计.md)第 5 节：RAPL 不可用的节点输出 `null`（前端显示"不可用"）；
> 站点设置 `show_power_public=false` 时整个字段省略，`show_cost_public=false` 时省略 `est_cost_month`（电价本身永不公开）。

### 1.3 历史曲线

`GET /api/v1/public/servers/:id/history?range=1h|6h|24h|7d`

- 访客默认可用 `1h/6h/24h`（原始采样降采样至 ≤240 点，字段白名单 `ts/cpu_pct/mem_used/disk_used/net_in_out_bps/power_w`）；`7d` 走小时聚合，默认仅登录可见（匿名返回 `401 {code:1002,msg:"长期历史需登录后查看"}`，管理端可配 `open_7d_history` 放开）；前端 `/server/:id` 节点详情页经 `api/monitor.js` 消费本接口（`normalizePoints` 统一短期/聚合字段）；
- 隐藏节点（`hidden=1`）返回 404。

### 1.4 实时流（SSE）

`GET /api/v1/public/stream`

- 首帧：`event: snapshot`，data 为 1.2 的全量列表；
- 之后：`event: update`，每轮采集后推送变化节点的最新数据（与 1.2 同结构）；
- 心跳 `: ping` 每 15s；客户端断线指数退避重连。

## 2. 初始化与认证

### 2.1 查询初始化状态

`GET /api/v1/admin/status` → `{ "initialized": false, "site_title": "BeaconTower" }`

前端据此决定展示"初始化向导"还是"登录页"。

### 2.2 初始化（仅当未初始化时可用；完成后永久 403）

`POST /api/v1/admin/setup`

```json
{ "username": "admin", "password": "<≥10位，须含大小写+数字+符号>" }
```

- 服务端做密码强度校验；Argon2id 哈希入库；
- 限速：每来源 IP 5 次/分钟；
- 可选加固：环境变量 `BEACON_SETUP_TOKEN` 设置后，请求必须携带 `{"setup_token": "..."}`（令牌打印在容器首次启动日志中），防止公网部署后被抢注。

### 2.3 登录 / 登出

```
POST /api/v1/admin/login    {"username": "...", "password": "..."}
  → Set-Cookie: bt_session=<token>; HttpOnly; SameSite=Strict; Path=/; Max-Age=86400
POST /api/v1/admin/logout   → 清除会话
```

- 失败限速 + 递增封禁（见 05 文档）；
- 响应不区分"用户名错误/密码错误"，统一"用户名或密码错误"。

### 2.4 CSRF

- 登录成功后下发 `bt_csrf` Cookie（可读）；所有非 GET 管理 API 必须带 `X-CSRF-Token` 头，双提交校验。

## 3. 节点管理（需会话）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/admin/servers` | 全量列表（**含** host/port/用户名/指纹/失败原因/私有备注，不含已加密的凭据明文） |
| POST | `/admin/servers` | 新建；`{"name","region"?,"tags","note_public","note_private","hidden","ssh":{host,port,username,auth_type,password?或 private_key,passphrase?}}`；`region` 留空 = 试连成功后按公网 IP 自动定位填充 |
| PUT | `/admin/servers/:id` | 编辑；凭据字段留空 = 保留原值 |
| DELETE | `/admin/servers/:id` | 删除（级联删除凭据/画像/指标） |
| PUT | `/admin/servers/:id/order` | 排序 `{"sort_order": n}` |
| PUT | `/admin/servers/:id/power-calibration` | 功耗校准：`{"base_load_w": 5.6}` 手动设定基础功耗（`base_load_source=manual`；RAPL 不可用节点返回 1001），见 doc/09 |
| POST | `/admin/servers/test` | **试连**：用提交的连接参数立即试连，成功则回读系统画像预览返回；失败返回 SSH 错误原因 |
| GET | `/admin/servers/:id/profile` | 强制刷新画像 |
| POST | `/admin/servers/:id/locate` | 强制重新执行公网 IP 定位（自动定位仅在 `region_source=auto` 时覆盖 region；返回 geo 结果，仅管理端） |

### 试连响应示例

```json
{ "code": 0, "data": {
    "ok": true, "latency_ms": 42,
    "profile": { "hostname": "web-hk-01", "os_name": "Ubuntu", "os_version": "24.04",
                 "kernel": "6.8.0-45", "arch": "x86_64", "cpu_model": "AMD EPYC ...",
                 "cpu_cores": 4, "mem_total": 4294967296, "virt": "kvm" },
    "geo": { "public_ip": "203.0.113.7", "country": "HK", "city": "Hong Kong" },
    "host_key_fp": "SHA256:xxxxxxxx..." } }
```

> 试连响应为管理端接口，包含 `public_ip` 仅供管理员核对；公开接口只输出 `region` 文案。

> 首次保存时记录 `host_key_fp`（TOFU）；管理端列表展示指纹，可开启严格校验模式（不匹配拒绝连接）。

## 4. 设置与审计

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/PUT | `/admin/settings` | 站点标题、采集间隔(≥5s)、历史保留天数、7d 历史是否对访客开放、公开功耗/电费开关、电价（doc/09）、Host Key 严格校验 |
| GET | `/admin/audit?page=` | 审计日志（`source_ip_hash` 仅展示哈希） |
| POST | `/admin/password` | 修改密码（需验证旧密码） |

## 5. 其他

- `GET /healthz` → 200（供 Docker 健康检查，不做鉴权、不进访问日志）；
- **NoRoute 兜底（单容器 SPA 回退，new-api 同款）**：静态资源优先；未命中且以 `/api` 开头的请求返回 JSON 404（`{"code":...}`，绝不返回 HTML）；其余未命中路径返回 `index.html`（`Cache-Control: no-cache`），由前端 history 路由接管（如刷新 `/admin/servers`）。

## 6. 错误码约定

| code | 含义 |
| --- | --- |
| 0 | 成功 |
| 1001 | 参数错误（msg 带字段说明） |
| 1002 | 未登录 / 会话过期 |
| 1003 | CSRF 校验失败 |
| 1004 | 已初始化，禁止重复初始化 |
| 1005 | 用户名或密码错误（登录） |
| 1006 | 请求过于频繁（限速触发） |
| 2001 | SSH 连接失败（msg 含分类：认证失败/超时/主机不可达） |
| 2002 | 节点不存在 |
| 5000 | 服务器内部错误 |
