# 16 · Cloudflare Tunnel 集成（第三穿透平台）

> 状态：已实现（v2.6）。客户端 `internal/frp/cloudflared.go`；绑定/隧道/托管链路
> 与 doc/13 的双平台框架完全复用，本文档只记录 Cloudflare 特有的设计与踩坑。

## 1. 背景与定位

面板已有 Sakura / ChmlFrp 两个国内穿透平台（doc/13），Cloudflare Tunnel 作为第三平台
接入，补足海外链路：

- 公网入口经 Cloudflare 边缘网络（自带 TLS、隐藏源站 IP、天然防 DDoS）；
- 与 tower.yoahoug.dev 现网（ops 上 `cf-tunnel` 容器）同一体系，面板可统一观测；
- 免费额度极宽裕（隧道数与流量均无硬上限），适合常驻服务而非临时穿透。

**与 frp 系平台的根本差异**：Cloudflare 是「云托管反向代理」而非「端口映射」——

| | Sakura / ChmlFrp | Cloudflare Tunnel |
| --- | --- | --- |
| 模型 | frpc 把 `local_ip:port` 映射到节点公网端口 | cloudflared 与 CF 边缘建 4 条 QUIC 连接，ingress 按域名分流 |
| 一个"Tunnel" | 一个端口映射 | 一个 cloudflared 连接器容器（内含任意多条 ingress 路由） |
| 在线状态 | 平台 API 明确给出 | tunnel 级 `status`（healthy/degraded/down/inactive）+ `connections` |
| 连接数 | frp 每条访客连接在源机一个 socket | 连接终结在 CF 边缘，源机 socket 口径**不适用** |
| 流量统计 | 平台 API 有 | 零信任免费版 API **无**流量数据（只有 GraphQL 企业版聚合，未接） |
| 配置 | frpc INI 逐隧道下发 | 远程托管：ingress 存 CF 云端，改完即生效无需重启 cloudflared |

**统一容器模型（v2.6 定稿，修正了初版"每条规则一条物理隧道"的设计）**：面板
只创建**一条专属物理隧道**（`beacontower-managed`，即"统一容器"），面板里新建/
删除「隧道」实际是往这条隧道里**追加/移除 ingress 路由**——一个 Tunnel 对应一个
cloudflared 容器，容器里挂多少个节点（域名路由）由面板统一管理。隧道列表、游客
看板、客户端托管三条既有链路按「一条 ingress 规则 = 一条面板隧道」展示归一，
但物理资源始终是单容器。

## 2. 鉴权模型：单 Token + 自动发现

**用户只需创建并粘贴一个 API Token**，其余参数由面板自动发现：

| 参数 | 获取方式 | 说明 |
| --- | --- | --- |
| API Token | 用户创建粘贴（唯一手工步骤） | 权限：`Account → Cloudflare Tunnel → Edit` +（建隧道时）`Zone → DNS → Edit` |
| Account ID | `GET /accounts` 自动发现 | token 自带账户级权限即可列出；多账户取第一个并记日志 |
| Zone ID | 首次同步按隧道 hostname 自动发现 | `GET /zones?name=<注册域>`；无 Zone Read 权限时静默降级（不写 CNAME、无流量数据） |

凭据 JSON（`account_id`/`token`/`zone_id`/`unified_tunnel_id` 四字段）序列化后
整体走 `frp_platform.token_enc` AES-256-GCM 加密落库，**复用现有凭据字段，零
schema 变更**（v13 迁移只为扩 kind 的 CHECK 约束）。`ParseCloudflaredCreds` 对
历史裸 token 形态做了兼容；cfEnhance 发现 zone / 统一容器后立即落库，后续同步
不再重复请求。

**统一容器 ID 的生命周期**：绑定时不创建——首次在面板新建节点时懒创建
（`EnsureUnifiedTunnel`，按名 `beacontower-managed` 幂等查找，找不到才建）并回写
进凭据；同步时凭据缺失会按名查找既有隧道补录。`CloudflaredClient.ManagedTunnelID`
由 Runner 从凭据注入，是写操作的唯一合法目标。

绑定弹窗标题旁有 **「?」帮助按钮**，点开是同风格指引弹窗：带 Cloudflare Token
创建页直达链接、权限配置两步说明（含「搜索框直接输 Cloudflare Tunnel」的提示）、
「Token 只显示一次」的注意事项。

> Cloudflare 没有 Zero Trust 之外的 OAuth 设备码/授权码路线可给第三方面板托管
> API 调用（OAuth 集中在 Access/SSO），「粘贴 Token」即为最简接入形态。

验活不走 `/user`（Tunnel Token 未必有 User Details 权限），直接拉
`GET /accounts/{id}/cfd_tunnel?is_deleted=false`，能列出隧道即凭据有效。

## 3. 协议坑（实现时必须注意；★ = E2E 真机实测抓到）

1. **外壳 `success:false` 可能配 HTTP 200**。与 ChmlFrp 同坑：只看状态码会把
   业务失败当成功。`cfEnvelope` 统一解析，`errors[0]` 转成 `apiError`。
2. **鉴权错误三形态**：HTTP 401/403、错误码 10000（认证失败）、9109（无权访问
   该资源）——三者都归 `ErrAuth`，前端统一弹「重新绑定」。
2. ★ **GET/PUT `/configurations` 的 ingress 嵌套在 `result.config` 下**（响应顶层
   是 tunnel_id/version/config），按顶层解析永远得空——症状是远程托管隧道全显示
   成"裸隧道行"（无 local_ip/remote），且写操作会把现有规则整体抹掉。
3. **cfd_tunnel 列表的 `connections` 是摘要**：每项含 `id/version/arch/conns_active/
   origin_ip`；`conns_active` 字段是隧道级活跃连接计数（connector 与边缘的连接）。
4. **远程托管 vs 本地托管**：`remote_config=true` 的隧道 ingress 存云端
   （`GET/PUT .../configurations`），API 可见可改；`false` 是本地 config.yml 托管，
   API **看不到规则**，面板归一为一条隧道并注明「本地 config.yml 托管」。
5. **2026-05 起创建 tunnel 必须带 `config_src: "cloudflare"`**，否则新账号默认
   走 token 远程托管时会出现配置源冲突。
6. **catch-all 规则**（无 hostname 的 `http_status:404`）必须始终在 ingress 末位；
   面板读写规则时都跳过/自动补上它。
7. **删除物理 tunnel 要求无活跃连接**，且删隧道体是危险动作；面板的「删除隧道」
   只删统一容器里的一条 ingress 规则 + 对应 CNAME，统一容器隧道体永远保留
   （它是其余节点的载体；下轮同步自动从镜像消失的是规则）。
8. ★ **运行令牌不是 JWT 三段式**，而是**单段标准 base64**（含 `+/=` padding；
   解码后是 `{"a":<account_id>,"t":<tunnel_id>}` 的 JSON）。校验正则必须接受
   `=` 结尾——初版按 JWT 三段式写，把合法 token 全拒了（部署报"令牌格式异常"）。
9. ★ **PUT configurations 丢 catch-all 会被 CF 拒绝**："The last ingress rule must
   match all URLs"。删规则/改规则重建数组时必须把末位空 hostname 规则原样带回
   （`rawFullIngress` 就是为这个存在的）。
10. ★ **DiscoverZone 不能"退回账户下第一个 zone"**：token 可见多个域名时会把
    CNAME 写进错误的 zone（实测 bt-e2e.yoahoug.dev 被写进 ahhhahh.cc.cd）。
    现在只做注册域精确匹配，查不到就静默降级（提示手动补记录），绝不建错。

## 4. 数据归一

### 4.1 隧道（cfd_tunnel → frp_tunnel）

| CF 字段 | 归一到 | 说明 |
| --- | --- | --- |
| `id` | `remote_id` 前缀 | 远程托管规则:`<tunnelID>:<hostname>`；本地托管:裸 ID |
| ingress `hostname` | `name` / `remote` | 面板里隧道名即对外域名 |
| ingress `service` | `local_ip:local_port` | `http://192.168.0.10:8091` → 192.168.0.10:8091 |
| `status=healthy/degraded` | `online=true` | degraded 附状态说明「连接器不健康」 |
| `status=down/inactive` | `online=false` | 说明「无在线连接器」 |
| `conns_active` | `conns` | connector 边缘连接数（口径与 frp 连接数不同，UI 不做换算） |
| `Remote` | `proto=https` | CF 隧道对外恒为 HTTPS（边缘终结 TLS） |

`remote_id` 带前缀的原因：`frp_tunnel` 唯一索引建在 `(platform_id, remote_id)`，
同一 tunnel 多条 ingress 规则必须行行唯一。

### 4.2 节点（connections → frp_node）

CF 无「接入节点」概念，把每条**远程托管**隧道的一个/多个 connector 归一为节点镜像
（`remote_id=tunnel ID`、`name=<tunnel> 连接器`），供游客看板「在用节点」展示。
本地托管隧道无 connections 数据，跳过。

### 4.3 指标口径（与 Sakura/ChmlFrp 对齐）

| 指标 | Sakura | ChmlFrp | Cloudflare | 对齐方式 |
| --- | --- | --- | --- | --- |
| 隧道在线 | 平台 API | 平台 API | tunnel status | 直接映射 |
| 连接数 | 本地 socket 计数（`local`） | 平台 `totalCurConns`（`platform`） | `conns_active` 各隧道之和（`platform`） | connector↔边缘活跃连接数，是平台真实值但语义与访客并发不同 |
| 今日流量 | 平台「本日消耗」 | 隧道当日进出合计 | **GraphQL Analytics** `sum(edgeResponseBytes)` 按 hostname 聚合 | 每轮同步回填 `today_up`/`traffic_day_used`（24h 口径） |
| 配额/剩余流量 | 平台套餐值 | 不限流量 | 无此概念 | 前端显示「—」/「不限」 |

**流量数据源**：`POST /client/v4/graphql`，查询 `httpRequestsAdaptiveGroups`
（官方确认全套餐开放的基础数据集），filter `clientRequestHTTPHost = <ingress 域名>`
+ 近 24h 时间窗——与「一条 ingress 规则 = 一条隧道」的归一严格对齐。
`edgeResponseBytes` 是边缘回源字节数，无法拆上下行，统一记 `today_up`（前端按
上+下行合计展示，游客页口径一致）。

**管线时序**：CF 的 `UserInfo` 不带流量值（恒 0），而 `SaveFRPPlatformProfile`
会把 `traffic_day_used` 覆盖为 0——因此流量回填放在画像落库**之后**
（`cfEnhance`），用独立的 `SetFRPPlatformDayTraffic`/`SetFRPTunnelTraffic`
覆盖写，与画像管线互不覆盖（与本地连接数计数 `local_conns` 的分离思路相同）。
GraphQL 查询失败只记日志，面板显示「—」，不影响主镜像。

## 5. 隧道写操作（只写统一容器）

所有写操作以 `ManagedTunnelID` 为唯一合法目标，**非统一容器的规则一律拒绝写**
（双保险：同步时锁 `lock_edit`/`lock_delete` 前端禁用按钮，客户端方法内二次校验）。

新建节点（`POST /frp/platforms/:id/tunnels`，kind=cloudflared）流程：

1. `EnsureUnifiedTunnel`：统一容器不存在则 `POST /cfd_tunnel`（name=
   `beacontower-managed`，config_src=cloudflare）并回写凭据（回写失败不回滚
   物理隧道，按名可复得）；存在则复用；
2. 读现有 ingress → 追加 `{hostname, service: http://<ip>:<port>}` → 保留 catch-all
   → `PUT .../configurations`（**不再新建物理隧道**——同一 hostname 已存在时直接报错）；
3. `POST /zones/:id/dns_records` 建 CNAME `<hostname> → <tunnelID>.cfargotunnel.com`
   （proxied=true，**失败不回滚**——节点已可用，提示手动补记录）。

- `TunnelInput.Name` 的语义在 CF 下是**完整域名**（ingress hostname），前端表单
  已按平台切换占位与校验；handler 对 CF 平台豁免「节点必填」校验（无节点可选）；
- `LocalIP` 留空/`localhost`/`127.0.0.1` 时改写为 `172.17.0.1`（Docker 网桥网关）：
  cloudflared 容器里的 localhost 是容器自己，与 doc/13 §13「必须 host 网络」是
  同一问题的两种解法（托管容器用 host 网络，ingress 侧用网关 IP 双保险）；
- 编辑 = `PUT .../configurations` 改对应规则的 service（只动统一容器）；
- 删除 = 移除该规则 + 清理指向 `<tunnelID>.cfargotunnel.com` 的 CNAME
  （统一容器隧道体永远保留——它是其余节点的载体）。

**独立专线只读边界（用户明确要求）**：ops 上的现役隧道（如 `New-api`，一服务
一线）**面板只读展示、绝不修改**。同步时凡不属于统一容器的规则都标
`lock_edit`/`lock_delete`（extra 注明「独立专线（只读）」）；即使前端被绕过，
`UpdateTunnel`/`DeleteOne` 的 `tunnelID != ManagedTunnelID` 校验也会拒绝。管理
这类专线以 CF 后台为准，面板侧重观测。

## 6. 客户端托管（syncCloudflared）

与 frpc 托管（doc/13 §14）同框架、三处不同：

| | frpc 托管 | cloudflared 托管 |
| --- | --- | --- |
| 配置 | INI 经 SSH stdin 落盘 0600 | **无配置文件**：tunnel token 经 API 实取，作 `docker run` 参数 |
| 镜像 | GHCR 自建 `beacontower-frpc-*` | 官方 `docker.io/cloudflare/cloudflared:latest`（国内节点拉取失败可设置项 `frp_image_cloudflared` 覆盖） |
| 配置过期 | 隧道改后需「同步配置」（脏标） | **无脏标概念**：ingress 云端即改即生效，容器不用动 |

容器名沿用 `beacontower-frpc-cloudflared-<server_id>`、`beacontower.managed=frpc`
标签、host 网络与 `--restart unless-stopped` 策略；动作/日志/删除链路完全复用。

安全取舍：token 出现在节点 `docker inspect` 的 Cmd 里（与 CF 官方 Dashboard 部署
方式一致）；容器日志回读时 grep 过滤 `eyJ|token` 防令牌进面板。

**一条托管 = 统一容器**：token 取自统一容器（`creds.UnifiedTunnelID`，缺失时从
托管隧道的 remote_id 前缀反解），一个 cloudflared 容器承载面板创建的全部节点；
ingress 云端即改即生效，新增/删除节点无需重启容器。独立专线有自己的连接器，
不归面板托管。

## 7. API 与前端

- 绑定：`POST /api/v1/admin/frp/cloudflared`（name/token，account_id/zone_id 可空
  由后端自动发现），先验活再落库；其余隧道/托管接口复用双平台的路由，按 kind 分派；
- `FrpView.vue`：绑定弹窗双形态（Sakura 密钥 / CF 单 Token）、标题旁「?」帮助
  弹窗（中英双语 CF 后台菜单对照与直达链接）、平台标签橙色 `frp-tag-cf`、
  建隧道弹窗按平台切换（CF：名=域名、无节点/协议/额外参数选择）、
  平台卡/用量瓦片对 CF 的「—」口径；
- `TunnelView.vue`（游客）：KIND_LABEL/KIND_ICON 加 cloudflared，今日流量列
  CF 有 GraphQL 数据源后正常展示。

### 7.1 前端三层转发的坑（E2E 实测踩到）

前端数据流是「视图 → Pinia store → API 门面 → HTTP 方法对象」**四层**：
`FrpView.vue` 调 `admin.frpBindCloudflared()`（store），store 转发
`adminClient.frpBindCloudflared`（带超时的门面对象），门面转发 `realAdmin.xxx`
（裸 HTTP 方法对象）。新增 API 方法时**四层都要加**，漏任何一层就是运行时
`xxx is not a function`。本次实测先漏了 store 转发、再漏了门面层（`realAdmin`
有方法但超时包装层没有），报错变量名会随漏层位置变化（`b.` → `i.`），
静态检查 chunk 内容全部「正常」，极易误判为缓存问题——修复后已全部补齐。

## 8. 单测

`internal/frp/cloudflared_test.go`（fixture 按真实响应结构构造，敏感值已替换）：

- ingress 归一：远程托管多规则 → 多条隧道、复合 remote_id、service 解析、
  catch-all 跳过、本地托管兜底一条；
- 统一容器模型：`ManagedTunnelID` 之外的规则锁编辑/删除（extra 标注独立专线）、
  写方法二次校验拒绝；`CreateTunnel` 往统一容器追加规则、物理隧道只懒创建一次
  （第二次创建复用，post 计数不变）；
- `success:false` + HTTP 200 必须报错；HTTP 401 归凭据失效语义；
- `SplitCFRemoteID` / 凭据往返编解码（含 `unified_tunnel_id` 与历史三字段兼容）/
  `defaultCFLocalIP`。

## 9. E2E 验证清单（真机待跑）

1. 绑定真实 CF 账号 → 同步展示 ops 现役 tunnel（healthy、规则数正确）；
2. 面板建测试隧道（新子域名）→ CF 后台可见 tunnel + ingress + CNAME；
3. 托管 cloudflared 容器到节点 → 容器 running、`docker logs` 见 4 条连接注册；
4. 公网 `curl -s https://<新子域名>/` 取回源站标记内容（对照 §13.2 的对照组思路）；
5. 编辑隧道本地端口 → 无需重启容器，公网立即生效（远程托管特性）；
6. 删除节点 → ingress 规则与 CNAME 消失，统一容器与其余节点不受影响；
7. 解绑平台 → 托管容器/记录清理，CF 侧资源不受影响。

## 10. 已知边界

- GraphQL 流量查询依赖 Zone ID（自动发现）与 token 的 Zone 级 Analytics 权限，
  两者都缺失时「今日流量」显示「—」；`registrableDomain` 取最后两段做注册域，
  未做 PSL（`com.cn` 等多段后缀的 zone 需域名与 zone 名完全一致才能命中）；
- 本地托管（config.yml）隧道在面板里只读展示，规则变更仍需登录节点改文件；
- 连接数口径是 connector↔边缘的活跃连接数，不是访客并发数，UI 不做等价暗示；
- 官方 cloudflared 镜像在 Docker Hub，被墙节点需通过设置项换镜像源
  （如 `ghcr.nju.edu.cn` 类代理或自建 mirror）。
