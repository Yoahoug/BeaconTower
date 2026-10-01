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

### 1.5 穿透摘要（游客版）

`GET /api/v1/public/frp`

> **白名单序列化**，与 1.2 同一套纪律：下面是该接口能出现的全部字段。
> 只回答访客关心的三件事——接了哪几个平台、隧道通不通、今天走了多少流量、在用的节点健不健康。
> **不下发**：账号画像（`username`/`uid`/`realname`/账号分组）、套餐信息（剩余流量、限速）、
> 内网拓扑（节点域名 `host`、隧道名与本地/公网端点、节点 `remote_id`）。
> 节点只回「被隧道挂载的那些」，平台全网节点表（Sakura 实测 71 条）不会出现在这里。
> 回归测试：`internal/handler/frp_public_test.go` 对响应体做禁止串扫描，漏字段即 CI 失败。

```json
{
  "code": 0,
  "data": {
    "platforms": [{
      "name": "Sakura", "kind": "natfrp", "online": true,
      "tunnel_total": 2, "tunnel_online": 2, "conns": 0,
      "traffic_today": 2294754205,
      "nodes": [{ "name": "长沙电信PLUS2", "group": "普通节点", "online": true, "load": 34.3, "uptime": 306200 }],
      "updated_at": 1790763685
    }],
    "summary": { "platform_total": 1, "tunnel_total": 2, "tunnel_online": 2, "conns": 0,
                 "traffic_today": 2294754205, "node_in_use": 2, "node_online": 2, "updated_at": 1790763685 }
  }
}
```

- `traffic_today`：Sakura 取账号级当日消耗；ChmlFrp 账号级无当日值，用其名下隧道当日进出之和兜底；
- `online`：平台最近一次同步是否成功（同步失败的平台为 `false`，不下发上游错误原文）；
- `uptime`：ChmlFrp 节点接口不报在线时长（恒 0），前端为 0 时不显示；
- `private_mode=true` 且未登录时返回 `401 {code:1002}`（与其余公开接口一致）；
- 前端 `/tunnels` 穿透状态页消费本接口（`stores/monitor.js` 的 `startFrp/stopFrp`，60s 独立轮询）。

### 1.6 WG 组网摘要（游客版，v2.4 新增）

`GET /api/v1/public/wg`

> **白名单序列化**，与 1.5 同一套纪律。只回答访客四件事——组网跑没跑、中心健不健康、
> 成员在线几个、今天/本月中转了多少流量。数据口径为 WG 巡检（5min ticker）回填的
> `wg_peer` 与 `wg_hub_traffic`，**公开端点不做任何 SSH 实时探测**。
> **不下发**：网段拓扑（`subnet`/`hub_ip`/`iface`/成员 `wg_ip`/中心 `endpoint`/`listen_port`）、
> 密钥材料（`public_key`/指纹/`has_keys`——公钥指纹也是拓扑情报）、
> 错误原文（巡检 `last_error` 内含 SSH host:port，只给健康布尔）、
> 设备成员名（Mac/iPhone 等私人设备画像，只给数量）；
> 隐藏节点成员沿用公开总览口径，不入公开成员表。
> 回归测试：`internal/handler/wg_public_test.go` 对响应体做禁止串扫描，漏字段即 CI 失败。

```json
{
  "code": 0,
  "data": {
    "initialized": true,
    "hubs": [
      { "name": "hub-aliyun", "is_active": true, "healthy": true, "month_rx": 1000, "month_tx": 2000 }
    ],
    "peers": [
      { "name": "home-nas", "online": true }
    ],
    "summary": { "hub_active": 1, "hub_healthy": 1, "member_total": 1, "member_online": 1,
                 "device_total": 1, "traffic_today": 3000, "traffic_month": 3000, "checked_at": 1790763685 }
  }
}
```

- `initialized=false`：尚未初始化组网，前端展示引导态（其余字段省略）；
- `hubs`：现役 + 未退役备援槽位；`healthy` 取巡检 `status == "ok"`（备援巡检失败 = `false`，不下发原因）；
- `month_rx/month_tx`：本月中转量（巡检按日差值累计，云厂商只对出方向计费时看 `month_tx`）；
- `traffic_today`：全部未退役中心当日 `rx+tx` 差值合计；`checked_at` 为最近一次巡检回写时刻；
- `private_mode=true` 且未登录时返回 `401 {code:1002}`（与其余公开接口一致）；
- 前端 `/mesh` 组网状态页消费本接口（`stores/monitor.js` 的 `startWg/stopWg`，60s 独立轮询）。

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
| 2010 | WG 预检/配置不通过 |
| 2011 | WG 执行失败 |
| 2012 | WG IP 冲突 |
| 2020 | 穿透平台不支持该操作（如对 ChmlFrp 调锁定） |
| 2021 | 穿透平台凭据失效，需重新授权 |
| 2022 | 上游穿透平台调用失败 |
| 5000 | 服务器内部错误 |

## 7. 内网穿透接口（/api/v1/admin/frp/*）

完整清单与错误语义见 [doc/13](./13-内网穿透平台管理设计.md)；此处只列路径索引：

```
GET     /frp/overview                          平台卡片 + 跨平台隧道 + 汇总
GET     /frp/nodes                             跨平台节点列表
POST    /frp/platforms                         绑定 Sakura（访问密钥）
PUT     /frp/platforms/:id                     改名
DELETE  /frp/platforms/:id                     解绑
GET     /frp/platforms/:id                     详情（账号 + 隧道 + 节点 + 用量快照）
POST    /frp/platforms/:id/sync                手动同步（?full=0 跳过节点）
GET     /frp/platforms/:id/flow                流量历史（?kind=day|week|month）
POST    /frp/platforms/:id/tunnels             新建隧道
GET     /frp/platforms/:id/subdomains          ChmlFrp 二级域名列表
GET     /frp/platforms/:id/subdomains/available 可用主域名
POST    /frp/platforms/:id/subdomains          新建解析
PUT     /frp/platforms/:id/subdomains          修改解析
DELETE  /frp/platforms/:id/subdomains          删除解析（domain/record 走查询参数）
PUT     /frp/tunnels/:id                       修改隧道
DELETE  /frp/tunnels/:id                       删除隧道
POST    /frp/tunnels/:id/lock                  锁定编辑/删除/迁移（仅 Sakura）
POST    /frp/tunnels/:id/migrate               迁移节点（仅 Sakura）
POST    /frp/tunnels/:id/offline               强制下线（仅 ChmlFrp）
POST    /frp/tunnels/:id/auth                  通过访问认证（仅 Sakura）
GET     /frp/tunnels/:id/config                下载 frpc 配置（裸文本附件）
GET     /frp/tunnels/:id/traffic               单隧道流量曲线
POST    /frp/chmlfrp/device                    发起 ChmlFrp 设备码授权
GET     /frp/chmlfrp/device/:sid               轮询授权状态
DELETE  /frp/chmlfrp/device/:sid               取消授权会话
```

> 全部外部调用带 30–45s 超时，前端 `retries: 0`（重试会重复建隧道）；
> `GET /frp/tunnels/:id/config` 返回裸文本附件而非 `{code,msg,data}` 包装
> （与 WG 凭证 zip 同一处理方式）。
>
> `GET /frp/nodes` 与 `GET /frp/platforms/:id` 的节点项都带 **`in_use`**：
> 平台节点表是全网节点（Sakura 实测 71 条），面板默认只展示被本账号隧道挂载的那些，
> 后端只提供标记、不替前端决定筛选口径。游客侧（§1.5）则直接只回在用节点。
