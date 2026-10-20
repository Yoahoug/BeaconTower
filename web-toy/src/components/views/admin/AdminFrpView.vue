<!-- ============================================================
     内网穿透管理（对齐 classic web/src/views/admin/FrpView.vue 的数据与操作）
     平台卡只做展示（各自特色指标），下方是跨平台归一化的一份内容：
     合并隧道表（平台列标注归属）、合并在用节点表（在用/未在用）、
     多平台同图流量历史、客户端托管（面板经 SSH 在节点起 frpc 容器）。
     平台差异以附加列与独立小节出现：Sakura 锁定/迁移/认证、
     ChmlFrp 下线/二级域名/设备码授权、Cloudflare 统一容器与只读专线。
     ============================================================ -->
<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-3 mb-2">
      <div class="space-y-1.5">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h2 class="text-xl font-black text-toy-black">内网穿透</h2>
          <span class="toy-badge bg-toy-yellow text-[11px] px-2.5 py-0.5">多平台聚合</span>
        </div>
        <p class="text-xs font-bold text-zinc-500">
          Sakura / ChmlFrp / Cloudflare 隧道统一管理 · 后台每 3 分钟同步（「刷新」= 立即向平台拉取）
        </p>
      </div>

      <div class="flex items-center gap-2 flex-wrap">
        <button
          @click="refresh"
          :disabled="busy"
          class="toy-btn px-3 py-1.5 bg-white hover:bg-toy-bg text-xs font-black rounded-xl cursor-pointer disabled:opacity-60"
          title="立即向各平台拉一次最新状态（轻量同步，不含节点列表）"
        >
          {{ busy ? '同步中…' : '↻ 刷新' }}
        </button>
        <button @click="openDevice(0)" class="toy-btn px-3 py-1.5 bg-white hover:bg-toy-bg text-xs font-black rounded-xl cursor-pointer">
          授权 ChmlFrp
        </button>
        <button @click="openBind('cloudflared')" class="toy-btn px-3 py-1.5 bg-white hover:bg-toy-bg text-xs font-black rounded-xl cursor-pointer">
          绑定 Cloudflare
        </button>
        <button @click="openBind('natfrp')" class="toy-btn px-3.5 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer">
          绑定 Sakura
        </button>
      </div>
    </div>

    <!-- 提示条 -->
    <div v-if="tip.text" class="toy-card p-3 flex items-center justify-between gap-3" :class="tip.error ? 'bg-red-50 border-2.5 border-red-400' : 'bg-toy-mint-light border-2.5 border-toy-black'">
      <span class="text-xs font-bold" :class="tip.error ? 'text-red-700' : 'text-toy-black'">{{ tip.error ? '⚠' : '✓' }} {{ tip.text }}</span>
      <button @click="tip.text = ''" class="text-xs font-black cursor-pointer">✕</button>
    </div>
    <div v-if="loadError" class="toy-card p-3 bg-red-50 border-2.5 border-red-400 flex items-center justify-between gap-3">
      <span class="text-xs font-bold text-red-700">⚠ {{ loadError }}</span>
      <button @click="refresh" class="text-xs font-black cursor-pointer underline">重试</button>
    </div>

    <!-- 空态：尚未接入平台 -->
    <div v-if="!loading && !platforms.length && !loadError" class="toy-card p-8 bg-white text-center space-y-3">
      <div class="text-sm font-black text-toy-black">尚未接入穿透平台</div>
      <p class="text-xs font-bold text-toy-muted">
        绑定 Sakura 访问密钥、授权 ChmlFrp 账号或绑定 Cloudflare API Token 后即可在此管理隧道
      </p>
      <div class="flex items-center justify-center gap-2 flex-wrap pt-1">
        <button @click="openBind('natfrp')" class="toy-btn px-3 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer">绑定 Sakura</button>
        <button @click="openDevice(0)" class="toy-btn px-3 py-1.5 bg-white text-xs font-black rounded-xl cursor-pointer">授权 ChmlFrp</button>
        <button @click="openBind('cloudflared')" class="toy-btn px-3 py-1.5 bg-white text-xs font-black rounded-xl cursor-pointer">绑定 Cloudflare</button>
      </div>
    </div>

    <template v-if="platforms.length">
      <!-- 平台页签 -->
      <div class="flex items-center gap-1.5 overflow-x-auto pb-1 scrollbar-none">
        <button
          v-for="t in TABS"
          :key="t.key"
          @click="activeTab = t.key"
          class="px-3 py-1 rounded-lg text-xs font-black border-2 border-toy-black transition-all shrink-0 cursor-pointer flex items-center gap-1.5"
          :class="activeTab === t.key ? 'bg-toy-black text-white shadow-toy-sm' : 'bg-white text-toy-black hover:bg-toy-yellow/30'"
        >
          {{ t.label }}
          <span class="px-1 rounded text-[10px] font-mono" :class="activeTab === t.key ? 'bg-white/20' : 'bg-toy-bg'">
            {{ t.key === 'all' ? allTunnels.length : allTunnels.filter((x) => x.platform_kind === t.key).length }}
          </span>
        </button>
      </div>

      <!-- 平台卡 -->
      <div>
        <h3 class="text-sm font-black text-toy-black mb-2">穿透平台</h3>
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-3.5">
          <div
            v-for="p in tabPlatforms"
            :key="p.id"
            class="toy-card p-4 bg-white space-y-3"
            :class="p.status !== 'ok' ? 'border-red-400' : ''"
          >
            <div class="flex items-start justify-between gap-2">
              <div class="flex items-center gap-2.5">
                <div class="w-9 h-9 rounded-lg border-2 border-toy-black flex items-center justify-center font-black text-[10px]" :class="kindAvatarClass(p.kind)">
                  {{ kindAvatarText(p.kind) }}
                </div>
                <div>
                  <div class="flex items-center gap-1.5 flex-wrap">
                    <h4 class="font-black text-sm text-toy-black">{{ p.name }}</h4>
                    <span class="toy-badge text-[10px]" :class="statusTag(p).cls">{{ statusTag(p).text }}</span>
                  </div>
                  <span class="text-[10px] font-bold text-toy-muted font-mono">
                    {{ kindLabel[p.kind] || p.kind }}<template v-if="p.username"> · {{ p.username }}</template>
                  </span>
                </div>
              </div>
            </div>

            <!-- 指标行：隧道在线 / 配额 / 平台特色指标 / 连接数 -->
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5 my-3">
              <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
                <div class="text-[11px] font-bold text-zinc-500">隧道在线</div>
                <div class="text-sm font-black font-mono text-toy-black mt-0.5">{{ p.tunnel_online }}<span class="text-zinc-400">/{{ p.tunnel_total }}</span></div>
              </div>
              <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
                <div class="text-[11px] font-bold text-zinc-500">配额</div>
                <div class="text-sm font-black font-mono text-toy-black mt-0.5">
                  {{ p.tunnel_quota > 0 ? `${p.tunnel_used}/${p.tunnel_quota}` : p.tunnel_quota < 0 ? '不限' : p.tunnel_used }}
                </div>
              </div>
              <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
                <div class="text-[11px] font-bold text-zinc-500">{{ kindStatLabel(p) }}</div>
                <div class="text-sm font-black font-mono text-toy-black truncate mt-0.5">{{ kindStatValue(p) }}</div>
              </div>
              <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
                <div class="text-[11px] font-bold text-zinc-500">{{ p.kind === 'natfrp' ? '签到' : p.kind === 'chmlfrp' ? '累计下载' : '连接器' }}</div>
                <div class="text-sm font-black font-mono text-toy-black truncate mt-0.5">{{ kindFourthValue(p) }}</div>
              </div>
            </div>

            <!-- 配额占用条 -->
            <div v-if="p.tunnel_quota && p.tunnel_quota > 0" class="h-2 w-full bg-toy-bg border border-toy-black rounded-full overflow-hidden my-2.5">
              <div class="h-full" :class="loadTone((Number(p.tunnel_used) || 0) / (p.tunnel_quota || 1) * 100).bar" :style="{ width: Math.min(100, (Number(p.tunnel_used) || 0) / (p.tunnel_quota || 1) * 100) + '%' }"></div>
            </div>

            <div v-if="p.last_error" class="text-xs font-bold text-red-600 break-words py-1">⚠ {{ p.last_error }}</div>
            <div class="text-[11px] font-bold text-zinc-500 py-1 flex items-center justify-between">
              <span>同步于 {{ p.last_sync_at ? agoText(p.last_sync_at) : '尚未同步' }}</span>
            </div>

            <div class="flex items-center gap-2 flex-wrap pt-2.5 border-t-2 border-toy-black/10">
              <button @click="syncOne(p)" :disabled="syncingId === p.id" class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-60 shadow-toy-xs">
                {{ syncingId === p.id ? '同步中…' : '同步' }}
              </button>
              <button v-if="p.kind === 'chmlfrp'" @click="openDevice(p.id)" class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer shadow-toy-xs">
                重新授权
              </button>
              <button v-if="p.kind === 'natfrp'" @click="openBind('natfrp')" class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer shadow-toy-xs">
                换密钥
              </button>
              <button v-if="p.kind === 'cloudflared'" @click="openBind('cloudflared')" class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer shadow-toy-xs">
                换 Token
              </button>
              <button @click="askDeletePlatform(p)" class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white text-red-600 hover:bg-red-50 cursor-pointer shadow-toy-xs">
                解绑
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Cloudflare 专属说明 -->
      <div v-if="cfPlatform && activeTab === 'cloudflared'" class="toy-card p-4 bg-white space-y-2">
        <h3 class="text-sm font-black text-toy-black">Cloudflare 的模型</h3>
        <p class="text-xs font-bold text-toy-muted leading-relaxed">
          一个 Tunnel = 一个 cloudflared 连接器容器，容器内可挂多条域名路由。面板用<b>统一容器</b>管理你新建的节点——
          「新建隧道」就是把一条域名路由挂进面板专属的 <code class="font-mono bg-toy-bg px-1 rounded">beacontower-managed</code> 隧道，
          托管时也只需要一个 cloudflared 容器，增删节点云端即生效、无需重启。
        </p>
        <p class="text-xs font-bold text-toy-muted leading-relaxed">
          你在 Cloudflare 后台手工建的独立专线（一服务一线）会以<b>只读</b>方式同步展示，面板不会改动它们；
          流量经 Cloudflare GraphQL Analytics 按域名聚合，Token 无 Analytics 权限时显示「—」。
        </p>
      </div>

      <!-- 账号用量（仅「全部」页签） -->
      <div v-if="activeTab === 'all'" class="toy-card p-5 bg-white space-y-4">
        <div class="flex items-center justify-between gap-2 flex-wrap">
          <h3 class="text-sm font-black text-toy-black">账号用量</h3>
          <span class="text-[10px] font-bold text-toy-muted">同列指标按平台口径换算：剩余流量 / 累计上传</span>
        </div>
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
          <div v-for="p in tabPlatforms" :key="`u-${p.id}`" class="p-3 bg-toy-bg border-2 border-toy-black rounded-xl space-y-1.5">
            <div class="flex items-center gap-1.5">
              <span class="w-5 h-5 rounded border border-toy-black flex items-center justify-center text-[9px] font-black" :class="kindAvatarClass(p.kind)">{{ kindAvatarText(p.kind) }}</span>
              <span class="text-xs font-black text-toy-black">{{ p.name }}</span>
            </div>
            <div class="flex justify-between text-[11px] font-bold"><span class="text-toy-muted">今日流量</span><span class="font-mono text-toy-black">{{ p.traffic_day_used ? formatBytes(p.traffic_day_used) : '—' }}</span></div>
            <div class="flex justify-between text-[11px] font-bold">
              <span class="text-toy-muted">{{ p.kind === 'natfrp' ? '剩余流量' : p.kind === 'cloudflared' ? '近24h边缘流量' : '累计上传/下载' }}</span>
              <span class="font-mono text-toy-black">
                {{ p.kind === 'natfrp'
                  ? (p.traffic_remain ? formatBytes(p.traffic_remain) : '—')
                  : p.kind === 'cloudflared' ? '平台不提供' : `${formatBytes(p.traffic_up)} / ${formatBytes(p.traffic_down)}` }}
              </span>
            </div>
            <div class="flex justify-between text-[11px] font-bold"><span class="text-toy-muted">限速</span><span class="font-mono text-toy-black">{{ p.speed_limit || (p.kind === 'cloudflared' ? '不限' : '—') }}</span></div>
            <div class="flex justify-between text-[11px] font-bold"><span class="text-toy-muted">上次同步</span><span class="font-mono text-toy-black">{{ p.last_sync_at ? fmtTs(p.last_sync_at) : '—' }}</span></div>
            <div v-if="p.realname" class="flex justify-between text-[11px] font-bold"><span class="text-toy-muted">实名 / 分组</span><span class="font-mono text-toy-black truncate">{{ p.realname }}<template v-if="p.group_name"> · {{ p.group_name }}</template></span></div>
          </div>
        </div>
      </div>

      <!-- 流量历史（仅「全部」页签） -->
      <div v-if="activeTab === 'all'" class="toy-card p-5 bg-white space-y-3">
        <div class="flex items-center justify-between gap-2">
          <h3 class="text-sm font-black text-toy-black">流量历史</h3>
          <button @click="loadTrend" class="px-2 py-1 rounded-lg text-[10px] font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer">↻ 重新加载</button>
        </div>
        <p v-if="trendLoading" class="text-xs font-bold text-toy-muted">加载中…</p>
        <p v-else-if="!trendSeries.length" class="text-xs font-bold text-toy-muted">
          暂无历史数据：平台侧的流量历史接口粒度较粗，点「重新加载」拉取；趋势图也依赖每轮同步的本地快照累积。
        </p>
        <ToyLineChart
          v-else
          :series="trendSeries"
          :labels="trendLabels"
          :height="220"
          :format-y="(v: number) => formatBytes(v)"
          :format-val="(v: number | null) => (v === null ? '—' : formatBytes(v))"
        />
      </div>

      <!-- 隧道表 -->
      <div class="toy-card p-5 bg-white space-y-4">
        <div class="flex items-center justify-between gap-2 flex-wrap">
          <h3 class="text-sm font-black text-toy-black">
            隧道 <span class="font-mono text-toy-muted">{{ tabTunnels.length }}</span>
          </h3>
          <button @click="openTunnelCreate" class="toy-btn px-3 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer">➕ 新建隧道</button>
        </div>

        <p v-if="!tabTunnels.length" class="text-xs font-bold text-toy-muted">暂无隧道：点上方「新建隧道」或到平台控制台创建后同步。</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead>
              <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                <th class="pb-2">状态</th>
                <th class="pb-2">名称</th>
                <th v-if="activeTab === 'all'" class="pb-2">平台</th>
                <th class="pb-2">节点</th>
                <th class="pb-2">本地</th>
                <th class="pb-2">公网</th>
                <th class="pb-2 text-right">连接数</th>
                <th class="pb-2 text-right">今日流量</th>
                <th class="pb-2 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-toy-gray">
              <tr v-for="t in tabTunnels" :key="t.id" class="hover:bg-toy-bg/50 transition-colors align-top">
                <td class="py-3">
                  <span class="toy-badge text-[10px]" :class="tunnelStatus(t).cls">{{ tunnelStatus(t).text }}</span>
                  <div v-if="t.status_reason" class="text-[10px] font-bold text-toy-muted mt-1 max-w-[140px]">{{ t.status_reason }}</div>
                </td>
                <td class="py-3">
                  <div class="flex items-center gap-1.5 flex-wrap">
                    <span class="px-1.5 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white font-mono">{{ protoLabel[t.proto] || t.proto }}</span>
                    <b class="text-toy-black">{{ t.name }}</b>
                    <span v-if="deployByTunnel[t.id]" class="toy-badge text-[9px] bg-toy-mint" :title="`由面板托管的客户端承载：${deployByTunnel[t.id].server_name} · ${deployByTunnel[t.id].container}`">面板托管</span>
                  </div>
                  <div v-if="t.extra" class="text-[10px] font-bold text-toy-muted mt-0.5">{{ t.extra }}</div>
                  <div v-if="deployByTunnel[t.id]" class="text-[10px] font-bold text-toy-muted mt-0.5">
                    {{ deployByTunnel[t.id].server_name }} · {{ deployByTunnel[t.id].status === 'running' ? '客户端运行中' : '客户端已停止' }}
                  </div>
                </td>
                <td v-if="activeTab === 'all'" class="py-3">
                  <span class="toy-badge text-[10px]" :class="kindTagClass(t.platform_kind)">{{ kindLabel[t.platform_kind] || t.platform_kind }}</span>
                </td>
                <td class="py-3 font-mono text-[11px] text-toy-black">{{ t.node_name || t.node_id || '—' }}</td>
                <td class="py-3 font-mono text-[11px] text-toy-black">{{ tunnelLocalText(t) || '—' }}</td>
                <td class="py-3 font-mono text-[11px] text-toy-black break-all">{{ t.remote || '—' }}</td>
                <td class="py-3 text-right font-mono text-[11px]">
                  <span v-if="(t.conns || 0) > 0" :title="`平台侧连接数 ${t.conns}`">{{ t.conns }}</span>
                  <span v-else-if="(t.local_conns || 0) > 0" class="text-toy-muted" title="面板在 frpc 所在节点实测的活跃连接数（Sakura 平台不提供连接数）">{{ t.local_conns }}</span>
                  <span v-else class="text-toy-muted">—</span>
                </td>
                <td class="py-3 text-right font-mono text-[11px]">
                  <span v-if="((t.today_up || 0) + (t.today_down || 0)) > 0">{{ formatBytes((t.today_up || 0) + (t.today_down || 0)) }}</span>
                  <span v-else class="text-toy-muted">—</span>
                </td>
                <td class="py-3">
                  <div class="flex items-center justify-end gap-1 flex-wrap">
                    <button
                      @click="openTunnelEdit(t)"
                      :disabled="isCfReadonly(t)"
                      :title="isCfReadonly(t) ? '该节点属于独立专线（非面板创建），仅同步展示' : ''"
                      class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
                    >编辑</button>
                    <button
                      v-if="deployByTunnel[t.id]"
                      @click="askReleaseTunnel(t)"
                      :disabled="deployByTunnel[t.id].status !== 'running'"
                      title="让面板托管的客户端停止占用这条隧道，交给其它机器/官方客户端接管"
                      class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-40"
                    >释放</button>
                    <details class="relative">
                      <summary class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer list-none">⋯</summary>
                      <div class="absolute right-0 z-30 mt-1 w-40 bg-white border-2 border-toy-black rounded-xl shadow-toy-md p-1.5 space-y-0.5">
                        <button @click="downloadConfig(t)" :disabled="downloadingId === t.id" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer disabled:opacity-50">
                          {{ downloadingId === t.id ? '下载中…' : '下载配置' }}
                        </button>
                        <button @click="openDeploy(t)" :disabled="isCfReadonly(t)" :title="isCfReadonly(t) ? '独立专线只读' : ''" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer disabled:opacity-40">托管到节点</button>
                        <button v-if="t.platform_kind === 'natfrp'" @click="openLock(t)" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer">锁定</button>
                        <button v-if="t.platform_kind === 'natfrp'" @click="openMigrate(t)" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer">迁移节点</button>
                        <button v-if="t.platform_kind === 'natfrp'" @click="authTunnel(t)" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer">访问认证</button>
                        <button v-if="t.platform_kind === 'chmlfrp'" @click="offlineTunnel(t)" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer">下线</button>
                        <button @click="askDeleteTunnel(t)" :disabled="isCfReadonly(t)" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold text-red-600 hover:bg-red-50 cursor-pointer disabled:opacity-40">删除隧道</button>
                      </div>
                    </details>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 客户端托管 -->
      <div class="toy-card p-5 bg-white space-y-4">
        <div class="flex items-center justify-between gap-2 flex-wrap">
          <h3 class="text-sm font-black text-toy-black">
            客户端托管 <span class="font-mono text-toy-muted">{{ deployments.length }}</span>
          </h3>
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-[10px] font-bold text-toy-muted hidden sm:inline">面板经 SSH 在节点上拉起 frpc 容器 · 隧道变更后需重新同步</span>
            <button @click="loadDeployments(true)" :disabled="deployLoading" class="px-2 py-1 rounded-lg text-[10px] font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-60">
              ↻ 刷新状态
            </button>
            <button @click="openDeploy(null)" class="toy-btn px-3 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer">➕ 部署客户端</button>
          </div>
        </div>

        <p v-if="deployLoading && !deployments.length" class="text-xs font-bold text-toy-muted">读取托管状态中…</p>
        <p v-else-if="!deployments.length" class="text-xs font-bold text-toy-muted">
          尚未托管客户端：隧道建好后不必再手动登录节点跑 frpc——选一个平台与节点，面板会拉取镜像、生成配置并在节点上起容器。
        </p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead>
              <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                <th class="pb-2 whitespace-nowrap">状态</th>
                <th class="pb-2">节点</th>
                <th class="pb-2">平台</th>
                <th class="pb-2">承载隧道</th>
                <th class="pb-2">容器 / 镜像</th>
                <th class="pb-2 whitespace-nowrap">上次同步</th>
                <th class="pb-2 text-right whitespace-nowrap">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-toy-gray">
              <tr v-for="d in deployments" :key="d.id" class="hover:bg-toy-bg/50 transition-colors align-top">
                <td class="py-3">
                  <span class="toy-badge text-[10px] whitespace-nowrap" :class="deployStatusTag(d).cls">{{ deployStatusTag(d).text }}</span>
                  <div v-if="d.last_error" class="text-[10px] font-bold text-red-600 mt-1 max-w-[160px] break-words" :title="d.last_error">{{ d.last_error }}</div>
                </td>
                <td class="py-3">
                  <b class="text-toy-black">{{ d.server_name || `#${d.server_id}` }}</b>
                  <div v-if="d.docker_ver" class="text-[10px] font-bold text-toy-muted">{{ dockerVerText(d.docker_ver) }}</div>
                </td>
                <td class="py-3"><span class="toy-badge text-[10px] whitespace-nowrap" :class="kindTagClass(d.platform_kind)">{{ kindLabel[d.platform_kind] || d.platform_kind }}</span></td>
                <td class="py-3">
                  <div class="flex flex-wrap gap-1 max-w-[200px]">
                    <span v-for="t in d.tunnels" :key="t.id" class="px-1.5 py-0.5 rounded text-[10px] font-bold border border-toy-black whitespace-nowrap" :class="t.missing ? 'bg-red-50 text-red-600 line-through' : 'bg-white'">
                      {{ protoLabel[t.proto || ''] || 'TCP' }} · {{ t.name }}
                    </span>
                  </div>
                </td>
                <td class="py-3">
                  <div class="max-w-[240px]">
                    <span class="font-mono text-[11px] text-toy-black break-all">{{ d.container }}</span>
                    <div class="text-[10px] font-bold text-toy-muted break-all" :title="`${d.image} · ${d.config_path}`">{{ d.image }} · {{ d.config_path }}</div>
                  </div>
                </td>
                <td class="py-3 text-[11px] font-bold text-toy-black whitespace-nowrap">{{ d.last_sync_at ? agoText(d.last_sync_at) : '—' }}</td>
                <td class="py-3">
                  <div class="flex items-center justify-end gap-1 flex-wrap">
                    <button @click="deploySync(d)" :disabled="deployOpsId === d.id" title="重新拉取隧道配置、重建容器（隧道增删改后用这个）" class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-50">
                      {{ deployOpsId === d.id ? '…' : '同步' }}
                    </button>
                    <details class="relative">
                      <summary class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer list-none">⋯</summary>
                      <div class="absolute right-0 z-30 mt-1 w-40 bg-white border-2 border-toy-black rounded-xl shadow-toy-md p-1.5 space-y-0.5">
                        <button @click="deployAction(d, 'restart')" :disabled="deployOpsId === d.id" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer disabled:opacity-50">重启容器</button>
                        <button v-if="d.status === 'running'" @click="deployAction(d, 'stop')" :disabled="deployOpsId === d.id" title="停止容器：同时把该客户端承载的隧道让出来" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer disabled:opacity-50">停止</button>
                        <button v-else @click="deployAction(d, 'start')" :disabled="deployOpsId === d.id" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer disabled:opacity-50">启动</button>
                        <button @click="openDeployLogs(d)" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer">日志</button>
                        <button @click="openDeployEdit(d)" title="增删该客户端承载的隧道（节点与平台不可改）" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer">改承载隧道</button>
                        <button @click="askDeleteDeploy(d)" class="w-full text-left px-2 py-1 rounded text-[11px] font-bold text-red-600 hover:bg-red-50 cursor-pointer">移除托管</button>
                      </div>
                    </details>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 节点表：在用 / 未在用折叠 -->
      <div class="toy-card p-5 bg-white space-y-4">
        <div class="flex items-center justify-between gap-2 flex-wrap">
          <h3 class="text-sm font-black text-toy-black">
            在用节点 <span class="font-mono text-toy-muted">{{ inUseNodes.length }}</span>
          </h3>
          <span class="text-[10px] font-bold text-toy-muted">隧道所落节点 · 随「同步」更新</span>
        </div>

        <p v-if="!allNodes.length" class="text-xs font-bold text-toy-muted">暂无节点数据：点平台卡上的「同步」拉取节点列表。</p>
        <template v-else>
          <p v-if="!inUseNodes.length" class="text-xs font-bold text-toy-muted">
            暂无在用节点：隧道还没有落到任何节点上；新建隧道或完成一次「同步」后自动出现。
          </p>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead>
                <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                  <th class="pb-2">状态</th>
                  <th class="pb-2">节点</th>
                  <th class="pb-2">平台</th>
                  <th class="pb-2">分组</th>
                  <th class="pb-2">能力</th>
                  <th class="pb-2 text-right">负载</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-toy-gray">
                <tr v-for="n in inUseNodes" :key="`u-${n.platform_id}-${n.id}`" class="hover:bg-toy-bg/50 transition-colors">
                  <td class="py-3"><span class="toy-badge text-[10px]" :class="n.online ? 'bg-toy-mint' : 'bg-gray-200 text-toy-muted'">{{ n.online ? '在线' : '离线' }}</span></td>
                  <td class="py-3">
                    <div class="font-bold text-toy-black">{{ n.name }}</div>
                    <div v-if="n.host" class="text-[10px] font-mono font-bold text-toy-muted">{{ n.host }}</div>
                  </td>
                  <td class="py-3"><span class="toy-badge text-[10px]" :class="kindTagClass(kindOfPlatform(n.platform_id))">{{ kindLabel[kindOfPlatform(n.platform_id)] || '—' }}</span></td>
                  <td class="py-3 text-[11px] font-bold text-toy-black">{{ n.group_name || '—' }}</td>
                  <td class="py-3">
                    <div class="flex flex-wrap gap-1">
                      <span v-for="c in capsText(n.caps)" :key="c" class="px-1.5 py-0.5 rounded text-[10px] font-bold border border-toy-black bg-white">{{ c }}</span>
                      <span v-if="!n.caps?.length" class="text-toy-muted">—</span>
                    </div>
                  </td>
                  <td class="py-3">
                    <div v-if="n.load" class="flex items-center justify-end gap-1.5">
                      <span class="font-mono text-[11px] font-black text-toy-black">{{ (Number(n.load) || 0).toFixed(1) }}%</span>
                      <span class="w-14 h-1.5 bg-toy-bg border border-toy-black rounded-full overflow-hidden">
                        <span class="block h-full" :class="loadTone(n.load).bar" :style="{ width: Math.min(100, Number(n.load) || 0) + '%' }"></span>
                      </span>
                    </div>
                    <span v-else class="text-toy-muted">—</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="idleNodes.length" class="pt-1">
            <button @click="idleExpanded = !idleExpanded" class="text-xs font-black text-toy-black underline decoration-dashed cursor-pointer">
              {{ idleExpanded ? '▾' : '▸' }} 未在用节点（{{ idleNodes.length }}）
              <span class="font-bold text-toy-muted no-underline">默认折叠，建隧道 / 迁移时在弹窗下拉中选择</span>
            </button>
            <div v-if="idleExpanded" class="overflow-x-auto mt-2">
              <table class="w-full text-left text-xs">
                <thead>
                  <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                    <th class="pb-2">状态</th>
                    <th class="pb-2">节点</th>
                    <th class="pb-2">平台</th>
                    <th class="pb-2">分组</th>
                    <th class="pb-2">能力</th>
                    <th class="pb-2 text-right">负载</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-toy-gray">
                  <tr v-for="n in idleNodes" :key="`i-${n.platform_id}-${n.id}`">
                    <td class="py-2.5"><span class="toy-badge text-[10px]" :class="n.online ? 'bg-toy-mint' : 'bg-gray-200 text-toy-muted'">{{ n.online ? '在线' : '离线' }}</span></td>
                    <td class="py-2.5">
                      <div class="font-bold text-toy-black">{{ n.name }}</div>
                      <div v-if="n.host" class="text-[10px] font-mono font-bold text-toy-muted">{{ n.host }}</div>
                    </td>
                    <td class="py-2.5"><span class="toy-badge text-[10px]" :class="kindTagClass(kindOfPlatform(n.platform_id))">{{ kindLabel[kindOfPlatform(n.platform_id)] || '—' }}</span></td>
                    <td class="py-2.5 text-[11px] font-bold text-toy-black">{{ n.group_name || '—' }}</td>
                    <td class="py-2.5">
                      <div class="flex flex-wrap gap-1">
                        <span v-for="c in capsText(n.caps)" :key="c" class="px-1.5 py-0.5 rounded text-[10px] font-bold border border-toy-black bg-white">{{ c }}</span>
                        <span v-if="!n.caps?.length" class="text-toy-muted">—</span>
                      </div>
                    </td>
                    <td class="py-2.5 text-right font-mono text-[11px] font-black text-toy-black">{{ n.load ? `${(Number(n.load) || 0).toFixed(1)}%` : '—' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </template>
      </div>

      <!-- ChmlFrp 免费二级域名 -->
      <div v-if="chmlPlatform && (activeTab === 'all' || activeTab === 'chmlfrp')" class="toy-card p-5 bg-white space-y-4">
        <div class="flex items-center justify-between gap-2 flex-wrap">
          <h3 class="text-sm font-black text-toy-black flex items-center gap-2">
            免费二级域名 <span class="font-mono text-toy-muted">{{ subs.list.length }}</span>
            <span class="toy-badge text-[10px] bg-toy-lavender">ChmlFrp</span>
          </h3>
          <button @click="openSubCreate" class="toy-btn px-3 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer">➕ 新建解析</button>
        </div>

        <div v-if="subs.error" class="p-3 bg-red-50 border-2.5 border-red-400 rounded-xl flex items-center justify-between gap-2">
          <span class="text-xs font-bold text-red-700">⚠ {{ subs.error }}</span>
          <button @click="loadAllSubdomains" class="text-xs font-black underline cursor-pointer">重试</button>
        </div>
        <p v-else-if="subs.loading && !subs.list.length" class="text-xs font-bold text-toy-muted">读取解析记录中…</p>
        <p v-else-if="!subs.list.length" class="text-xs font-bold text-toy-muted">
          暂无解析记录：ChmlFrp 提供免费二级域名，可用于 HTTP(S) 隧道的绑定域名。
        </p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead>
              <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                <th class="pb-2">主机记录</th>
                <th class="pb-2">主域名</th>
                <th class="pb-2">类型</th>
                <th class="pb-2">目标</th>
                <th class="pb-2">TTL</th>
                <th class="pb-2 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-toy-gray">
              <tr v-for="sd in subs.list" :key="`${sd.record}.${sd.domain}`">
                <td class="py-3 font-mono text-[11px] font-black text-toy-black">{{ sd.record }}</td>
                <td class="py-3 text-[11px] font-bold text-toy-black">{{ sd.domain }}</td>
                <td class="py-3"><span class="px-1.5 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white font-mono">{{ sd.type }}</span></td>
                <td class="py-3 font-mono text-[11px] text-toy-black break-all">{{ sd.target }}</td>
                <td class="py-3 text-[11px] font-bold text-toy-black">{{ sd.ttl }}</td>
                <td class="py-3">
                  <div class="flex items-center justify-end gap-1">
                    <button @click="openSubEdit(sd)" class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer">编辑</button>
                    <button @click="askDeleteSubdomain(sd)" class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white text-red-600 hover:bg-red-50 cursor-pointer">删除</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>

    <!-- ===== 弹窗：绑定 Sakura / Cloudflare ===== -->
    <div v-if="bindModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 relative" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="text-xl">{{ bindModal.kind === 'natfrp' ? '🌸' : '☁️' }}</span>
            <div>
              <h3 class="text-base font-black text-toy-black">绑定 {{ kindLabel[bindModal.kind] }}</h3>
              <p class="text-[10px] font-bold text-toy-muted">
                {{ bindModal.kind === 'natfrp' ? '填入 Sakura 访问密钥，自动同步隧道与账号用量' : '填入 Cloudflare API Token（Account/Zone 由面板自动发现）' }}
              </p>
            </div>
          </div>
          <button @click="closeModal" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs cursor-pointer">✕</button>
        </div>

        <div class="mt-4 space-y-3">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">平台显示名</label>
            <input v-model="bindModal.name" type="text" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black" />
          </div>
          <div>
            <label class="block text-xs font-black text-toy-black mb-1 flex items-center gap-1.5">
              {{ bindModal.kind === 'natfrp' ? '访问密钥' : 'API Token' }}
              <button v-if="bindModal.kind === 'cloudflared'" @click="cfHelpModal = true" class="w-4 h-4 rounded-full border border-toy-black text-[9px] font-black cursor-pointer" title="Token 获取步骤">?</button>
            </label>
            <input v-model="bindModal.token" type="password" placeholder="粘贴平台令牌…" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black font-mono" />
          </div>
          <p v-if="bindModal.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ bindModal.error }}</p>
          <div class="pt-1 flex items-center justify-end gap-2">
            <button @click="closeModal" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
            <button @click="submitBind" :disabled="bindModal.busy" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
              {{ bindModal.busy ? '验证中…' : '保存并授权' }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：Cloudflare Token 获取指引 ===== -->
    <div v-if="cfHelpModal" class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black">如何获取 Cloudflare API Token</h3>
          <button @click="cfHelpModal = false" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <ol class="text-xs font-bold text-toy-muted space-y-1.5 list-decimal pl-4">
          <li>打开 <a class="underline text-toy-black" href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noopener noreferrer">dash.cloudflare.com/profile/api-tokens</a></li>
          <li>Create Token → 选择 <b>Create Custom Token</b></li>
          <li>权限（Permissions）按需勾选：
            <div class="pl-2 pt-1 space-y-0.5 font-mono text-[10px]">
              <div>Account · Cloudflare Tunnel · Edit</div>
              <div>Zone · DNS · Edit（用面板建域名路由时需要）</div>
              <div>Account · Account Analytics · Read（要流量统计时需要）</div>
            </div>
          </li>
          <li>Account Resources / Zone Resources 选择你的账号与域名</li>
          <li>创建后复制 Token（只显示一次），回到面板粘贴保存</li>
        </ol>
        <p class="text-[10px] font-bold text-toy-muted">
          说明：Account ID 与 Zone ID 由面板自动发现，无需手工填写；缺 Analytics 权限只是「流量」列显示「—」，不影响隧道管理。
        </p>
      </div>
    </div>

    <!-- ===== 弹窗：ChmlFrp 设备码授权 ===== -->
    <div v-if="deviceModal" class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black">{{ deviceModal.reuseId ? '重新授权 ChmlFrp' : '授权 ChmlFrp' }}</h3>
          <button @click="closeDevice" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>

        <p v-if="deviceModal.status === 'starting'" class="text-xs font-bold text-toy-muted">正在发起设备码授权…</p>
        <template v-else-if="deviceModal.status === 'pending'">
          <p class="text-xs font-bold text-toy-muted">在浏览器打开下方链接，登录 ChmlFrp 并输入用户码完成授权：</p>
          <div class="p-3 bg-toy-bg border-2 border-toy-black rounded-xl space-y-2 text-center">
            <div class="text-2xl font-black font-mono tracking-widest text-toy-black">{{ deviceModal.userCode || '————' }}</div>
            <a v-if="deviceModal.verifyUrl" :href="deviceModal.verifyUrl" target="_blank" rel="noopener noreferrer" class="text-xs font-black underline break-all">{{ deviceModal.verifyUrl }}</a>
          </div>
          <p class="text-[10px] font-bold text-toy-muted">
            剩余有效期约 {{ deviceModal.secondsLeft }} 秒 · 面板每 3 秒轮询一次，授权成功后自动完成绑定与首次同步。
          </p>
        </template>
        <p v-else-if="deviceModal.status === 'ok'" class="text-xs font-bold text-toy-black">✓ 授权成功</p>
        <p v-else class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ deviceModal.error || '授权失败，请重新发起' }}</p>

        <div class="pt-1 flex items-center justify-end gap-2">
          <button v-if="deviceModal.status === 'pending' && deviceModal.verifyUrl" @click="openVerifyUrl" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">打开授权页</button>
          <button @click="closeDevice" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">关闭</button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：建/改隧道 ===== -->
    <div v-if="tunnelModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 space-y-3 max-h-[92vh] overflow-y-auto" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black">{{ tunnelModal.mode === 'create' ? '新建隧道' : '编辑隧道' }}</h3>
          <button @click="closeModal" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>

        <div v-if="tunnelModal.mode === 'create'">
          <label class="block text-xs font-black text-toy-black mb-1">目标平台</label>
          <select v-model="tunnelModal.platformId" @change="onCreatePlatformChange" class="w-full px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none">
            <option v-for="p in platforms" :key="p.id" :value="p.id">{{ p.name }}（{{ kindLabel[p.kind] || p.kind }}）</option>
          </select>
        </div>
        <p v-else class="text-xs font-bold text-toy-muted">所属平台：<b class="text-toy-black">{{ tunnelModal.platformName }}</b>（平台不可改）</p>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">
            {{ tunnelModalIsCf ? 'Ingress 域名' : '隧道名' }}<span class="text-red-600 font-black"> *</span>
          </label>
          <input v-model="tunnelModal.name" type="text" :disabled="tunnelModalIsCf && tunnelModal.mode === 'edit'" :placeholder="tunnelModalIsCf ? 'app.example.com' : '例如 nas-web-drive'" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black disabled:opacity-60" />
          <p v-if="tunnelModalIsCf && tunnelModal.mode === 'edit'" class="text-[10px] font-bold text-toy-muted mt-1">编辑时域名不可改（只支持改本地地址 / 端口）；换域名请删除后重建。</p>
          <p v-else-if="tunnelModalIsCf" class="text-[10px] font-bold text-toy-muted mt-1">即对外访问的完整域名（须已在 Cloudflare 托管），本地服务由下方「本地地址 / 本地端口」决定。</p>
          <p v-if="tunnelModalIsChml" class="text-[10px] font-bold text-toy-muted mt-1">ChmlFrp 隧道名只允许字母、数字与下划线（不能带连字符、中文或空格）。</p>
        </div>

        <div class="grid gap-2" :class="tunnelModalIsCf ? 'grid-cols-1' : 'grid-cols-2'">
          <div v-if="!tunnelModalIsCf">
            <label class="block text-xs font-black text-toy-black mb-1">协议</label>
            <select v-model="tunnelModal.proto" class="w-full px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none">
              <option value="tcp">TCP</option>
              <option value="udp">UDP</option>
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">本地端口</label>
            <input v-model="tunnelModal.localPort" type="number" min="1" max="65535" placeholder="8080" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black font-mono" />
          </div>
        </div>

        <div class="grid gap-2" :class="tunnelModalIsCf ? 'grid-cols-1' : 'grid-cols-2'">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">本地地址</label>
            <input v-model="tunnelModal.localIp" type="text" placeholder="127.0.0.1" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black font-mono" />
          </div>
          <div v-if="!tunnelModalIsCf">
            <label class="block text-xs font-black text-toy-black mb-1">公网端口（0=平台分配）</label>
            <input v-model="tunnelModal.remotePort" type="number" min="0" max="65535" placeholder="0" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black font-mono" />
          </div>
        </div>

        <div v-if="!tunnelModalIsCf">
          <label class="block text-xs font-black text-toy-black mb-1">
            绑定域名<span v-if="tunnelModal.proto === 'http' || tunnelModal.proto === 'https'" class="text-red-600 font-black"> *</span>
          </label>
          <input v-model="tunnelModal.domain" type="text" placeholder="例如 nas.example.com" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black font-mono" />
        </div>

        <div v-if="!tunnelModalIsCf">
          <label class="block text-xs font-black text-toy-black mb-1">{{ tunnelModal.mode === 'create' ? '接入节点（必选）' : '接入节点（留空不修改）' }}</label>
          <select v-model="tunnelModal.nodeId" class="w-full px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none">
            <option value="">— 请选择 —</option>
            <option v-for="n in tunnelModal.nodes" :key="n.remote_id" :value="n.remote_id">
              {{ n.name }}{{ n.group_name ? ` · ${n.group_name}` : '' }}{{ n.online ? '' : '（离线）' }}
            </option>
          </select>
        </div>

        <div v-if="!tunnelModalIsCf">
          <label class="block text-xs font-black text-toy-black mb-1">备注 / 附加说明</label>
          <input v-model="tunnelModal.note" type="text" placeholder="这条隧道给谁用、指向什么服务" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black" />
        </div>

        <p v-if="tunnelModalIsCf" class="text-[10px] font-bold text-toy-muted">Cloudflare 规则：一条 Ingress 域名 = 一个对外服务，所有节点共用统一容器（cloudflared）；创建时自动写入 ingress 规则并补 DNS CNAME（需 Zone ID 权限）。本地地址留空 / localhost / 127.0.0.1 时按 Docker 网桥网关（172.17.0.1）处理。</p>

        <p v-if="tunnelModal.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ tunnelModal.error }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="closeModal" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitTunnel" :disabled="tunnelModal.busy" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ tunnelModal.busy ? '提交中…' : tunnelModal.mode === 'create' ? '确定创建' : '保存修改' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：迁移节点 ===== -->
    <div v-if="migrateModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black">迁移隧道节点</h3>
          <button @click="closeModal" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <p class="text-xs font-bold text-toy-muted">把「{{ migrateModal.name }}」迁移到另一个 Sakura 节点（仅 Sakura 支持）。</p>
        <select v-model="migrateModal.nodeId" class="w-full px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none">
          <option value="">— 请选择目标节点 —</option>
          <option v-for="n in migrateModal.nodes" :key="n.remote_id" :value="n.remote_id">{{ n.name }}{{ n.group_name ? ` · ${n.group_name}` : '' }}</option>
        </select>
        <p v-if="migrateModal.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ migrateModal.error }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="closeModal" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitMigrate" :disabled="migrateModal.busy" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ migrateModal.busy ? '提交中…' : '确认迁移' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：锁定设置（Sakura） ===== -->
    <div v-if="lockModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black">锁定设置</h3>
          <button @click="closeModal" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <p class="text-xs font-bold text-toy-muted">锁定「{{ lockModal.name }}」的平台侧操作（仅 Sakura 支持）。</p>
        <label class="flex items-center gap-2 text-xs font-black text-toy-black cursor-pointer">
          <input v-model="lockModal.edit" type="checkbox" class="w-4 h-4 accent-black" /> 锁定编辑
        </label>
        <label class="flex items-center gap-2 text-xs font-black text-toy-black cursor-pointer">
          <input v-model="lockModal.del" type="checkbox" class="w-4 h-4 accent-black" /> 锁定删除
        </label>
        <label class="flex items-center gap-2 text-xs font-black text-toy-black cursor-pointer">
          <input v-model="lockModal.migrate" type="checkbox" class="w-4 h-4 accent-black" /> 锁定迁移
        </label>
        <p v-if="lockModal.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ lockModal.error }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="closeModal" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitLock" :disabled="lockModal.busy" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ lockModal.busy ? '提交中…' : '保存设置' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：二级域名 ===== -->
    <div v-if="subModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black">{{ subModal.mode === 'create' ? '新建解析' : '修改解析' }}</h3>
          <button @click="closeModal" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <div>
          <label class="block text-xs font-black text-toy-black mb-1">主域名</label>
          <select v-model="subModal.domain" :disabled="subModal.mode === 'edit'" class="w-full px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none disabled:opacity-60">
            <option v-for="d in subs.domains" :key="d.domain" :value="d.domain">{{ d.domain }}</option>
            <option v-if="subModal.mode === 'edit' && !subs.domains.some((x) => x.domain === subModal?.domain)" :value="subModal.domain">{{ subModal.domain }}</option>
          </select>
        </div>
        <div>
          <label class="block text-xs font-black text-toy-black mb-1">主机记录</label>
          <input v-model="subModal.record" :disabled="subModal.mode === 'edit'" type="text" placeholder="例如 nas" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black font-mono disabled:opacity-60" />
        </div>
        <div class="grid grid-cols-2 gap-2">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">记录类型</label>
            <select v-model="subModal.type" :disabled="subModal.mode === 'edit'" class="w-full px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none disabled:opacity-60">
              <option>A</option><option>AAAA</option><option>CNAME</option><option>SRV</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">TTL</label>
            <select v-model="subModal.ttl" class="w-full px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none">
              <option v-for="t in SUB_TTLS" :key="t" :value="t">{{ t }}</option>
            </select>
          </div>
        </div>
        <div>
          <label class="block text-xs font-black text-toy-black mb-1">目标地址</label>
          <input v-model="subModal.target" type="text" placeholder="例如 node.example.com" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black font-mono" />
        </div>
        <div>
          <label class="block text-xs font-black text-toy-black mb-1">备注</label>
          <input v-model="subModal.remarks" type="text" class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none text-toy-black" />
        </div>
        <p v-if="subModal.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ subModal.error }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="closeModal" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitSub" :disabled="subModal.busy" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ subModal.busy ? '提交中…' : '保存' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：部署 / 调整客户端托管 ===== -->
    <div v-if="deployModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 space-y-3 max-h-[92vh] overflow-y-auto" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <div>
            <h3 class="text-base font-black text-toy-black">{{ deployModal.editId ? '调整托管客户端' : '部署客户端到节点' }}</h3>
            <p class="text-[10px] font-bold text-toy-muted">面板经 SSH 拉取镜像、生成配置并在节点上起 frpc 容器</p>
          </div>
          <button @click="closeModal" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">平台</label>
          <select v-model="deployModal.platformId" :disabled="!!deployModal.editId" @change="onDeployPlatformChange" class="w-full px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none disabled:opacity-60">
            <option v-for="p in platforms" :key="p.id" :value="p.id">{{ p.name }}（{{ kindLabel[p.kind] || p.kind }}）</option>
          </select>
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">节点（仅显示已录入 SSH 凭据的节点）</label>
          <div class="flex items-center gap-2">
            <select v-model="deployModal.serverId" :disabled="!!deployModal.editId" class="flex-1 px-3 py-2 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none disabled:opacity-60">
              <option value="">— 请选择节点 —</option>
              <option v-for="s in deployServerOptions" :key="s.id" :value="String(s.id)" :disabled="deployTakenIds.includes(s.id)">
                {{ s.name }}{{ deployTakenIds.includes(s.id) ? '（已托管该平台）' : '' }}{{ sshReady(s) ? '' : '（无 SSH 凭据）' }}
              </option>
            </select>
            <button @click="probeDocker" :disabled="!deployModal.serverId || deployModal.probing" class="px-2 py-1.5 rounded-lg text-[10px] font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-50 whitespace-nowrap">
              {{ deployModal.probing ? '检测中…' : '检测 Docker' }}
            </button>
          </div>
          <div v-if="deployModal.docker" class="mt-1.5 text-[10px] font-bold" :class="deployModal.docker.present && deployModal.docker.daemon_ok ? 'text-emerald-700' : 'text-amber-700'">
            <template v-if="deployModal.docker.present && deployModal.docker.daemon_ok">
              ✓ Docker {{ deployModal.docker.version || '已就绪' }}
            </template>
            <template v-else>
              ⚠ {{ deployModal.docker.present ? `Docker 守护进程不可用：${deployModal.docker.err || '未知原因'}` : '节点未安装 Docker：可勾选下方「自动安装 Docker」' }}
            </template>
          </div>
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">承载隧道（同一隧道只应由一台机器承载，已占用的不可选）</label>
          <div class="max-h-40 overflow-y-auto p-2 bg-toy-bg border-2 border-toy-black rounded-xl space-y-1">
            <label v-for="t in deployTunnelPool" :key="t.id" class="flex items-center gap-2 text-xs font-bold cursor-pointer" :class="deployTunnelTaken(t) ? 'opacity-40 cursor-not-allowed' : ''">
              <input type="checkbox" :value="t.id" v-model="deployModal.tunnelIds" :disabled="deployTunnelTaken(t)" class="w-3.5 h-3.5 accent-black" />
              <span class="font-mono text-[10px] px-1 rounded border border-toy-black bg-white">{{ protoLabel[t.proto] || t.proto }}</span>
              <span class="text-toy-black">{{ t.name }}</span>
              <span class="text-toy-muted font-mono">→ {{ t.local_ip }}:{{ t.local_port }}</span>
              <span v-if="deployTunnelTaken(t)" class="text-[10px] text-toy-muted">（已被 {{ deployByTunnel[t.id]?.server_name }} 承载）</span>
            </label>
            <p v-if="!deployTunnelPool.length" class="text-[11px] font-bold text-toy-muted">该平台下暂无隧道</p>
          </div>
        </div>

        <label class="flex items-center gap-2 text-xs font-black text-toy-black cursor-pointer">
          <input v-model="deployModal.installDocker" type="checkbox" class="w-4 h-4 accent-black" /> 自动安装 Docker（缺 Docker 时勾选，安装需几分钟）
        </label>

        <p v-if="deployModal.busy" class="text-[11px] font-bold text-toy-muted">已等待 {{ deployModal.elapsed }} 秒（装 Docker / 拉镜像可能需要几分钟）…</p>
        <p v-if="deployModal.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ deployModal.error }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="closeModal" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitDeploy" :disabled="deployModal.busy" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ deployModal.busy ? '部署中…' : deployModal.editId ? '重新同步' : '开始部署' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：托管客户端日志 ===== -->
    <div v-if="deployLogs" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-3xl w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <div>
            <h3 class="text-base font-black text-toy-black">容器日志</h3>
            <p class="text-[10px] font-bold text-toy-muted font-mono">{{ deployLogs.container }}</p>
          </div>
          <button @click="deployLogs = null" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <p v-if="deployLogs.loading" class="text-xs font-bold text-toy-muted">读取日志中…</p>
        <p v-else-if="deployLogs.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ deployLogs.error }}</p>
        <pre v-else class="p-3 bg-toy-black text-toy-mint text-[11px] font-mono rounded-xl max-h-[60vh] overflow-auto whitespace-pre-wrap">{{ deployLogs.text || '（无输出）' }}</pre>
        <div class="flex items-center justify-end gap-2">
          <button @click="reloadDeployLogs" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">↻ 重新加载</button>
          <button @click="deployLogs = null" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">关闭</button>
        </div>
      </div>
    </div>

    <!-- ===== 危险操作确认 ===== -->
    <div v-if="confirmState" class="fixed inset-0 z-[70] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <h3 class="text-base font-black text-toy-black">{{ confirmState.title }}</h3>
        <p class="text-xs font-bold text-toy-muted leading-relaxed">{{ confirmState.message }}</p>
        <p v-if="confirmState.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ confirmState.error }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="confirmState = null" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button
            @click="runConfirm"
            :disabled="confirmState.busy"
            class="toy-btn px-4 py-1.5 rounded-xl text-xs font-black cursor-pointer disabled:opacity-60"
            :class="confirmState.danger ? 'bg-toy-pink text-white' : 'bg-toy-yellow'"
          >
            {{ confirmState.busy ? '执行中…' : confirmState.confirmText || '确认' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import type {
  AdminServerItem,
  FRPDeployItem,
  FRPDockerStatus,
  FRPNodeItem,
  FRPPlatformDetailData,
  FRPPlatformItem,
  FRPSubdomainItem,
  FRPTunnelItem,
} from '../../../types/backend';
import { api } from '../../../services/api';
import { formatBytes, formatAgo } from '../../../services/format';
import ToyLineChart from '../../charts/ToyLineChart.vue';

// 设备码轮询间隔：官方建议 5s，这里 3s 让用户等待感更短（后端已按平台 interval 节流）
const DEVICE_POLL_MS = 3000;
const SUB_TTLS = ['1分钟', '2分钟', '5分钟', '10分钟', '15分钟', '30分钟', '1小时', '2小时', '5小时', '12小时', '1天'];

const kindLabel: Record<string, string> = { natfrp: 'Sakura', chmlfrp: 'ChmlFrp', cloudflared: 'Cloudflare' };
const protoLabel: Record<string, string> = { tcp: 'TCP', udp: 'UDP', http: 'HTTP', https: 'HTTPS' };
const TABS = [
  { key: 'all', label: '全部' },
  { key: 'natfrp', label: 'Sakura' },
  { key: 'chmlfrp', label: 'ChmlFrp' },
  { key: 'cloudflared', label: 'Cloudflare' },
];
const CAPS_LABEL: Record<string, string> = {
  http: 'HTTP 建站', https: 'HTTPS 建站', web: '建站', udp: 'UDP',
  create: '可创建', mainland: '内地', nodefense: '无防', defense: '有防御',
  private: '私有', auth: '强制认证', beta: 'BETA', vip: 'VIP', ipv6: 'IPv6',
};

const platforms = ref<FRPPlatformItem[]>([]);
const details = ref<Record<number, FRPPlatformDetailData>>({});
const allNodes = ref<FRPNodeItem[]>([]);
const deployments = ref<FRPDeployItem[]>([]);
const loading = ref(false);
const loadError = ref('');
const busy = ref(false);
const activeTab = ref('all');
const idleExpanded = ref(false);

const tip = ref({ text: '', error: false });
let tipTimer: ReturnType<typeof setTimeout> | undefined;
function toast(text: string, error = false) {
  tip.value = { text, error };
  clearTimeout(tipTimer);
  tipTimer = setTimeout(() => (tip.value.text = ''), 4000);
}

function agoText(ts: number): string {
  return formatAgo(Math.max(0, Math.floor(Date.now() / 1000) - ts));
}
function fmtTs(ts: number): string {
  const d = new Date(ts * 1000);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

// ---------- 派生 ----------
const allTunnels = computed(() => tunnels.value);
const tunnels = ref<FRPTunnelItem[]>([]);

const tabPlatforms = computed(() =>
  activeTab.value === 'all' ? platforms.value : platforms.value.filter((p) => p.kind === activeTab.value),
);
const tabTunnels = computed(() =>
  activeTab.value === 'all' ? allTunnels.value : allTunnels.value.filter((t) => t.platform_kind === activeTab.value),
);
const inUseNodes = computed(() => allNodes.value.filter((n) => n.in_use));
const idleNodes = computed(() => allNodes.value.filter((n) => !n.in_use));
const chmlPlatform = computed(() => platforms.value.find((p) => p.kind === 'chmlfrp') || null);
const cfPlatform = computed(() => platforms.value.find((p) => p.kind === 'cloudflared') || null);

// 隧道 → 承载它的托管（用于「面板托管」标记与「释放占用」）
const deployByTunnel = computed(() => {
  const m: Record<number, FRPDeployItem> = {};
  for (const d of deployments.value) for (const id of d.tunnel_ids || []) m[id] = d;
  return m;
});

function kindOfPlatform(platformId: number): string {
  return platforms.value.find((p) => p.id === platformId)?.kind || '';
}

// ---------- 加载 ----------
async function loadOverview() {
  try {
    const r = await api.getFRPOverview();
    platforms.value = r.platforms ?? [];
    tunnels.value = r.tunnels ?? [];
    loadError.value = '';
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : '读取穿透概览失败';
  }
}

async function loadDetails() {
  const map: Record<number, FRPPlatformDetailData> = {};
  await Promise.all(
    platforms.value.map(async (p) => {
      try {
        map[p.id] = await api.getFRPPlatformDetail(p.id);
      } catch {
        /* 单个平台详情失败不影响整页：节点/隧道仍可由 overview 补齐 */
      }
    }),
  );
  details.value = map;
}

async function loadNodes() {
  try {
    const r = await api.getFRPNodes();
    allNodes.value = r.nodes ?? [];
  } catch {
    // 节点列表失败时退回平台详情里的节点
    const flat: FRPNodeItem[] = [];
    for (const d of Object.values(details.value)) flat.push(...(d.nodes || []));
    allNodes.value = flat;
  }
}

async function loadDeployments(refreshLive = false) {
  deployLoading.value = true;
  try {
    const r = await api.getFRPDeployments(refreshLive);
    deployments.value = r.deployments ?? [];
  } catch (e) {
    if (refreshLive) toast(e instanceof Error ? e.message : '读取托管状态失败', true);
  } finally {
    deployLoading.value = false;
  }
}

async function loadAllSubdomains() {
  const p = chmlPlatform.value;
  if (!p) {
    subs.value = { loading: false, list: [], domains: [], error: '' };
    return;
  }
  subs.value = { ...subs.value, loading: true, error: '' };
  try {
    const [a, b] = await Promise.all([api.getFRPSubdomains(p.id), api.getFRPAvailableDomains(p.id)]);
    subs.value = { loading: false, list: a.subdomains ?? [], domains: b.domains ?? [], error: '' };
  } catch (e) {
    subs.value = { ...subs.value, loading: false, error: e instanceof Error ? e.message : '域名信息加载失败' };
  }
}

// 「刷新」= 现在就向平台拉一次最新状态（轻量同步 full=false：只要账号与隧道，
// 不带节点列表），再读本地快照。只读本地库会出现「平台已恢复、面板还显示离线」的
// 假象；单个平台失败不阻断其它平台（原因由平台卡的 last_error 呈现）。
async function refresh() {
  busy.value = true;
  try {
    for (const p of platforms.value) {
      try {
        await api.syncFRPPlatform(p.id, false);
      } catch {
        /* 见上：错误留给平台卡 */
      }
    }
    await loadOverview();
    await Promise.all([loadDetails(), loadDeployments()]);
    await Promise.all([loadNodes(), loadAllSubdomains()]);
  } finally {
    busy.value = false;
  }
}

const syncingId = ref(0);
async function syncOne(p: FRPPlatformItem) {
  syncingId.value = p.id;
  try {
    await api.syncFRPPlatform(p.id, true);
    await loadOverview();
    await loadDetails();
    await loadNodes();
    toast(`「${p.name}」同步完成`);
  } catch (e) {
    toast(e instanceof Error ? e.message : '同步失败', true);
  } finally {
    syncingId.value = 0;
  }
}

// 页面停留期间每 60s 只重读本地快照（不发平台请求）：后端每 3 分钟同步一次
let idleTimer = 0;
async function idleReload() {
  if (busy.value || document.hidden) return;
  await loadOverview();
  await loadDeployments();
}

// ---------- 展示口径 ----------
function statusTag(p: FRPPlatformItem): { cls: string; text: string } {
  if (p.status === 'ok') return { cls: 'bg-toy-mint', text: '正常' };
  if (p.status === 'unbound') return { cls: 'bg-red-200 text-red-800', text: '需重新授权' };
  if (p.status === 'error') return { cls: 'bg-amber-200 text-amber-900', text: '同步异常' };
  return { cls: 'bg-toy-lavender text-toy-black', text: '未同步' };
}
function tunnelStatus(t: FRPTunnelItem): { cls: string; text: string } {
  if (t.online) return { cls: 'bg-toy-mint', text: '在线' };
  if (t.status === 'banned') return { cls: 'bg-red-200 text-red-800', text: '封禁' };
  if (t.status && t.status !== 'normal' && t.status !== 'unknown') return { cls: 'bg-amber-200 text-amber-900', text: t.status };
  // 离线本身不是错误（可能只是没人跑客户端），但要看得见
  return { cls: 'bg-toy-gray text-toy-black', text: '离线' };
}
function deployStatusTag(d: FRPDeployItem): { cls: string; text: string } {
  if (d.dirty) return { cls: 'bg-amber-200 text-amber-900', text: '待同步' };
  if (d.status === 'running') return { cls: 'bg-toy-mint', text: '运行中' };
  if (d.status === 'stopped') return { cls: 'bg-toy-lavender text-toy-black', text: '已停止' };
  if (d.status === 'missing') return { cls: 'bg-amber-200 text-amber-900', text: '容器丢失' };
  if (d.status === 'error') return { cls: 'bg-red-200 text-red-800', text: '异常' };
  return { cls: 'bg-toy-gray text-toy-black', text: '待部署' };
}
// docker_ver 有时是版本号（"29.2.0"），有时是 `docker version` 原文（"Docker version 29…"）：
// 原文已自带前缀时不再叠一层 "Docker Docker version"。
function dockerVerText(v: string): string {
  return /^docker/i.test(v.trim()) ? v.trim() : `Docker ${v.trim()}`;
}
function kindTagClass(kind: string): string {
  if (kind === 'chmlfrp') return 'bg-toy-lavender text-toy-black';
  if (kind === 'cloudflared') return 'bg-orange-200 text-toy-black';
  return 'bg-sky-200 text-toy-black';
}
function kindAvatarText(kind: string): string {
  if (kind === 'chmlfrp') return 'CM';
  if (kind === 'cloudflared') return 'CF';
  return 'SA';
}
function kindAvatarClass(kind: string): string {
  if (kind === 'chmlfrp') return 'bg-toy-lavender';
  if (kind === 'cloudflared') return 'bg-orange-200';
  return 'bg-sky-200';
}
function loadTone(load: number): { bar: string; text: string } {
  if (load >= 80) return { bar: 'bg-red-500', text: 'text-red-600' };
  if (load >= 50) return { bar: 'bg-amber-400', text: 'text-amber-700' };
  return { bar: 'bg-toy-mint', text: 'text-toy-black' };
}
function capsText(caps: string[] | undefined): string[] {
  return (caps || []).map((c) => CAPS_LABEL[c] || c);
}

// 平台第三列的「特色指标」：Sakura 剩余流量 / CF 无统计 / ChmlFrp 累计上传
function kindStatLabel(p: FRPPlatformItem): string {
  return p.kind === 'natfrp' ? '剩余流量' : p.kind === 'cloudflared' ? '流量统计' : '累计上传';
}
function kindStatValue(p: FRPPlatformItem): string {
  if (p.kind === 'natfrp') return p.traffic_remain ? formatBytes(p.traffic_remain) : '—';
  if (p.kind === 'cloudflared') return '—';
  return formatBytes(p.traffic_up);
}
function kindFourthValue(p: FRPPlatformItem): string {
  if (p.kind === 'natfrp') {
    const signed = Boolean(p.profile?.sign_signed);
    return signed ? `签 ${Number(p.profile?.sign_days) || 0} 天` : '未签';
  }
  if (p.kind === 'chmlfrp') return formatBytes(p.traffic_down);
  return p.conns ? String(p.conns) : '—';
}

// Cloudflare 统一容器本体（无 ingress 规则的物理隧道行）不是可访问的服务：
// 没有本地端点（local_ip 空、local_port=0），本地列显示「—」而不是「:0」。
function isCfContainerRow(t: FRPTunnelItem): boolean {
  return t.platform_kind === 'cloudflared' && !t.remote && !t.local_ip && !t.local_port;
}
function tunnelLocalText(t: FRPTunnelItem): string {
  if (isCfContainerRow(t)) return '';
  return `${t.local_ip || '127.0.0.1'}:${t.local_port}`;
}
// CF 独立专线（非面板统一容器）只读：平台同步时锁了编辑/删除的（运维手工建的专线），
// 操作按钮一律禁用——判据取 lock_edit/lock_delete，不是隧道名。
function isCfReadonly(t: FRPTunnelItem): boolean {
  return t.platform_kind === 'cloudflared' && (t.lock_edit || t.lock_delete);
}

// ---------- 弹窗基础 ----------
type ConfirmState = {
  title: string;
  message: string;
  confirmText?: string;
  danger?: boolean;
  busy?: boolean;
  error?: string;
  run?: () => Promise<void>;
} | null;
const confirmState = ref<ConfirmState>(null);

function closeModal() {
  bindModal.value = null;
  tunnelModal.value = null;
  migrateModal.value = null;
  lockModal.value = null;
  subModal.value = null;
  deployModal.value = null;
}

// ---------- 绑定平台 ----------
const bindModal = ref<{ kind: string; name: string; token: string; busy: boolean; error: string } | null>(null);
const cfHelpModal = ref(false);

function openBind(kind = 'natfrp') {
  bindModal.value = {
    kind,
    name: kind === 'cloudflared' ? 'Cloudflare' : 'Sakura',
    token: '',
    busy: false,
    error: '',
  };
}

async function submitBind() {
  const m = bindModal.value;
  if (!m) return;
  if (!m.token.trim()) {
    m.error = m.kind === 'cloudflared' ? '请填写 API Token（点标题旁「?」查看获取步骤）' : '请填写访问密钥';
    return;
  }
  m.busy = true;
  m.error = '';
  try {
    if (m.kind === 'cloudflared') await api.saveFRPPlatformToken({ kind: 'cloudflared' }, m.token.trim());
    else await api.saveFRPPlatformToken({ kind: 'natfrp' }, m.token.trim());
    closeModal();
    await refresh();
    toast(`${kindLabel[m.kind]} 绑定成功`);
  } catch (e) {
    m.error = e instanceof Error ? e.message : '绑定失败';
  } finally {
    if (bindModal.value) bindModal.value.busy = false;
  }
}

// ---------- ChmlFrp 设备码授权 ----------
const deviceModal = ref<{
  reuseId: number;
  sessionId: string;
  status: string;
  userCode: string;
  verifyUrl: string;
  secondsLeft: number;
  error: string;
} | null>(null);
let pollTimer = 0;

async function openDevice(reuseId = 0) {
  deviceModal.value = { reuseId, sessionId: '', status: 'starting', userCode: '', verifyUrl: '', secondsLeft: 0, error: '' };
  try {
    const s = await api.startFRPDevice(reuseId);
    if (!deviceModal.value) return;
    deviceModal.value.sessionId = s.session_id;
    deviceModal.value.userCode = s.user_code;
    deviceModal.value.verifyUrl = s.verify_url;
    deviceModal.value.secondsLeft = s.expires_in || 600;
    deviceModal.value.status = 'pending';
    startPolling(s.session_id);
  } catch (e) {
    if (deviceModal.value) {
      deviceModal.value.status = 'error';
      deviceModal.value.error = e instanceof Error ? e.message : '发起授权失败';
    }
  }
}

function startPolling(sessionId: string) {
  if (pollTimer) window.clearInterval(pollTimer);
  pollTimer = window.setInterval(async () => {
    const m = deviceModal.value;
    if (!m) return;
    if (m.secondsLeft > 0) m.secondsLeft -= DEVICE_POLL_MS / 1000;
    try {
      const r = await api.pollFRPDevice(sessionId);
      if (r.status === 'ok') {
        window.clearInterval(pollTimer);
        pollTimer = 0;
        closeDevice();
        await refresh();
        toast(`ChmlFrp 授权成功${r.username ? `（${r.username}）` : ''}`);
        return;
      }
      if (r.status !== 'pending') {
        window.clearInterval(pollTimer);
        pollTimer = 0;
        m.status = r.status === 'expired' ? 'expired' : 'error';
        m.error = r.error || '授权失败，请重新发起';
      }
    } catch (e) {
      window.clearInterval(pollTimer);
      pollTimer = 0;
      m.status = 'error';
      m.error = e instanceof Error ? e.message : '轮询失败';
    }
  }, DEVICE_POLL_MS);
}

async function closeDevice() {
  const m = deviceModal.value;
  if (pollTimer) {
    window.clearInterval(pollTimer);
    pollTimer = 0;
  }
  deviceModal.value = null;
  if (m?.sessionId && m.status === 'pending') {
    try {
      await api.cancelFRPDevice(m.sessionId);
    } catch {
      /* 会话可能已过期 */
    }
  }
}

function openVerifyUrl() {
  if (deviceModal.value?.verifyUrl) window.open(deviceModal.value.verifyUrl, '_blank', 'noopener');
}

// ---------- 建 / 改隧道 ----------
const tunnelModal = ref<{
  mode: 'create' | 'edit';
  id: number;
  platformId: number;
  platformName: string;
  platformKind: string;
  nodes: FRPNodeItem[];
  name: string;
  proto: string;
  nodeId: string;
  localIp: string;
  localPort: string;
  remotePort: string;
  domain: string;
  note: string;
  busy: boolean;
  error: string;
} | null>(null);

const tunnelModalIsChml = computed(() => tunnelModal.value?.platformKind === 'chmlfrp');
const tunnelModalIsCf = computed(() => tunnelModal.value?.platformKind === 'cloudflared');

function openTunnelCreate() {
  const first = platforms.value[0];
  tunnelModal.value = {
    mode: 'create',
    id: 0,
    platformId: first?.id || 0,
    platformName: first?.name || '',
    platformKind: first?.kind || '',
    nodes: details.value[first?.id || 0]?.nodes || [],
    name: '',
    proto: 'tcp',
    nodeId: '',
    localIp: '127.0.0.1',
    localPort: '',
    remotePort: '',
    domain: '',
    note: '',
    busy: false,
    error: '',
  };
}

// 建隧道时切换平台 → 节点候选跟随该平台的节点镜像（Cloudflare 无节点概念）
function onCreatePlatformChange() {
  const m = tunnelModal.value;
  if (!m || m.mode !== 'create') return;
  const p = platforms.value.find((x) => x.id === m.platformId);
  m.platformName = p?.name || '';
  m.platformKind = p?.kind || '';
  m.nodeId = '';
  // Cloudflare 没有协议概念（ingress service 固定 http://），切到 CF 时复位，
  // 避免把别的平台上选过的 http/https 一起带过去
  if (p?.kind === 'cloudflared') m.proto = 'tcp';
  m.nodes = details.value[m.platformId]?.nodes || [];
}

function openTunnelEdit(t: FRPTunnelItem) {
  const sec = details.value[t.platform_id];
  // remote 可能是「域名」也可能是「端口」：编辑时按形态回填到对应字段
  const remoteIsPort = /^\d+$/.test(t.remote || '');
  tunnelModal.value = {
    mode: 'edit',
    id: t.id,
    platformId: t.platform_id,
    platformName: t.platform_name,
    platformKind: t.platform_kind,
    nodes: sec?.nodes || [],
    name: t.name,
    proto: t.proto,
    nodeId: '',
    localIp: t.local_ip || '127.0.0.1',
    localPort: String(t.local_port || ''),
    remotePort: remoteIsPort ? t.remote : '',
    domain: remoteIsPort ? '' : t.remote || '',
    note: t.extra || '',
    busy: false,
    error: '',
  };
}

async function submitTunnel() {
  const m = tunnelModal.value;
  if (!m) return;
  const isCf = tunnelModalIsCf.value;
  // Cloudflare 下 name 即 ingress 域名，协议/公网端口/绑定域名/备注都没有平台语义
  // （后端 CF 分支只消费 name/local_ip/local_port），在这里统一归一，避免把界面上
  // 隐藏字段里的陈留值发给后端
  const payload = {
    name: m.name.trim(),
    proto: isCf ? 'tcp' : m.proto,
    node_id: m.nodeId,
    local_ip: m.localIp.trim(),
    local_port: Number(m.localPort) || 0,
    remote_port: isCf ? 0 : Number(m.remotePort) || 0,
    domain: isCf ? '' : m.domain.trim(),
    note: isCf ? '' : m.note.trim(),
    extra: '',
  };
  if (!payload.name) {
    m.error = isCf ? '请填写 Ingress 域名' : '请填写隧道名';
    return;
  }
  if (!payload.local_port) {
    m.error = '请填写本地端口';
    return;
  }
  if (payload.local_port > 65535) {
    m.error = '本地端口必须在 1-65535 之间';
    return;
  }
  if ((m.proto === 'http' || m.proto === 'https') && !payload.domain && !isCf) {
    m.error = 'HTTP(S) 隧道必须填写绑定域名';
    return;
  }
  if (isCf && !payload.name.includes('.')) {
    m.error = 'Cloudflare 的 Ingress 域名须是完整域名（如 app.example.com）';
    return;
  }
  if (tunnelModalIsChml.value && !/^[A-Za-z0-9_]+$/.test(payload.name)) {
    m.error = 'ChmlFrp 隧道名只允许字母、数字与下划线（不能带连字符、中文或空格）';
    return;
  }
  m.busy = true;
  m.error = '';
  try {
    if (m.mode === 'create') {
      if (!isCf && !payload.node_id) {
        m.error = '请选择节点';
        return;
      }
      await api.createFRPTunnelFull(m.platformId, payload);
      toast('隧道已创建');
    } else {
      await api.updateFRPTunnel(m.id, payload);
      toast('隧道已更新');
    }
    closeModal();
    await loadOverview();
    await loadDeployments();
  } catch (e) {
    m.error = e instanceof Error ? e.message : '提交失败';
  } finally {
    if (tunnelModal.value) tunnelModal.value.busy = false;
  }
}

// ---------- 迁移 / 锁定 / 认证 / 下线 ----------
const migrateModal = ref<{ id: number; name: string; nodes: FRPNodeItem[]; nodeId: string; busy: boolean; error: string } | null>(null);
const lockModal = ref<{ id: number; name: string; edit: boolean; del: boolean; migrate: boolean; busy: boolean; error: string } | null>(null);

function openMigrate(t: FRPTunnelItem) {
  migrateModal.value = {
    id: t.id,
    name: t.name,
    nodes: details.value[t.platform_id]?.nodes || [],
    nodeId: '',
    busy: false,
    error: '',
  };
}
async function submitMigrate() {
  const m = migrateModal.value;
  if (!m) return;
  if (!m.nodeId) {
    m.error = '请选择目标节点';
    return;
  }
  m.busy = true;
  m.error = '';
  try {
    await api.migrateFRPTunnel(m.id, m.nodeId);
    closeModal();
    await loadOverview();
    toast('迁移已提交');
  } catch (e) {
    m.error = e instanceof Error ? e.message : '迁移失败';
  } finally {
    if (migrateModal.value) migrateModal.value.busy = false;
  }
}

function openLock(t: FRPTunnelItem) {
  lockModal.value = {
    id: t.id,
    name: t.name,
    edit: t.lock_edit,
    del: t.lock_delete,
    migrate: t.lock_migrate,
    busy: false,
    error: '',
  };
}
async function submitLock() {
  const m = lockModal.value;
  if (!m) return;
  m.busy = true;
  m.error = '';
  try {
    await api.lockFRPTunnel(m.id, { edit: m.edit, delete: m.del, migrate: m.migrate });
    closeModal();
    await loadOverview();
    toast('锁定设置已更新');
  } catch (e) {
    m.error = e instanceof Error ? e.message : '操作失败';
  } finally {
    if (lockModal.value) lockModal.value.busy = false;
  }
}

async function offlineTunnel(t: FRPTunnelItem) {
  try {
    await api.toggleFRPTunnel(t.id, true);
    await loadOverview();
    toast('已强制下线');
  } catch (e) {
    toast(e instanceof Error ? e.message : '操作失败', true);
  }
}

async function authTunnel(t: FRPTunnelItem) {
  try {
    const r = await api.authFRPTunnel(t.id, '');
    toast(`已授权访问：${r.ip || '当前来源 IP'}`);
  } catch (e) {
    toast(e instanceof Error ? e.message : '操作失败', true);
  }
}

function askDeleteTunnel(t: FRPTunnelItem) {
  confirmState.value = {
    title: '删除隧道',
    message: `确认在 ${kindLabel[t.platform_kind] || t.platform_kind} 平台删除隧道「${t.name}」？该操作会直接调用平台接口，不可撤销。`,
    confirmText: '删除',
    danger: true,
    busy: false,
    run: async () => {
      await api.deleteFRPTunnel(t.id);
      await loadOverview();
      await loadDeployments();
      toast('隧道已删除');
    },
  };
}

function askDeletePlatform(p: FRPPlatformItem) {
  confirmState.value = {
    title: '解绑平台',
    message: `确认解绑「${p.name}」？将删除面板内的隧道镜像与用量历史，平台侧的隧道不会被删除。`,
    confirmText: '解绑',
    danger: true,
    busy: false,
    run: async () => {
      await api.deleteFRPPlatform(p.id);
      await loadOverview();
      await loadDetails();
      await loadNodes();
      await loadAllSubdomains();
      toast('平台已解绑');
    },
  };
}

function askDeleteSubdomain(sd: FRPSubdomainItem) {
  confirmState.value = {
    title: '删除解析记录',
    message: `确认删除解析 ${sd.record}.${sd.domain}？该操作会直接调用平台接口。`,
    confirmText: '删除',
    danger: true,
    busy: false,
    run: async () => {
      await api.deleteFRPSubdomain(chmlPlatform.value?.id || 0, sd.domain, sd.record);
      await loadAllSubdomains();
      toast('解析已删除');
    },
  };
}

async function runConfirm() {
  const c = confirmState.value;
  if (!c?.run) return;
  c.busy = true;
  c.error = '';
  try {
    await c.run();
    confirmState.value = null;
  } catch (e) {
    c.busy = false;
    c.error = e instanceof Error ? e.message : '操作失败';
  }
}

// ---------- 配置下载 ----------
const downloadingId = ref(0);
async function downloadConfig(t: FRPTunnelItem) {
  downloadingId.value = t.id;
  try {
    await api.downloadFRPTunnelConfig(t);
  } catch (e) {
    toast(e instanceof Error ? e.message : '下载失败', true);
  } finally {
    downloadingId.value = 0;
  }
}

// ---------- 客户端托管 ----------
const deployLoading = ref(false);
const deployOpsId = ref(0);
const deployServers = ref<AdminServerItem[]>([]);
const deployModal = ref<{
  editId: number;
  platformId: number;
  serverId: string;
  tunnelIds: number[];
  installDocker: boolean;
  docker: FRPDockerStatus | null;
  probing: boolean;
  busy: boolean;
  elapsed: number;
  error: string;
} | null>(null);
const deployLogs = ref<{ id: number; container: string; loading: boolean; text: string; error: string } | null>(null);

function sshReady(s: AdminServerItem): boolean {
  return Boolean(s?.ssh?.ssh_ready);
}

const deployServerOptions = computed(() =>
  [...deployServers.value].sort((a, b) => Number(sshReady(b)) - Number(sshReady(a))),
);

const deployTunnelPool = computed(() =>
  allTunnels.value.filter((t) => t.platform_id === deployModal.value?.platformId),
);

function deployTunnelTaken(t: FRPTunnelItem): boolean {
  const d = deployByTunnel.value[t.id];
  return Boolean(d) && d.id !== (deployModal.value?.editId || 0);
}

function deployTakenServerIds(platformId: number, exceptId = 0): number[] {
  return deployments.value
    .filter((d) => d.platform_id === platformId && d.id !== exceptId)
    .map((d) => d.server_id);
}
const deployTakenIds = computed(() =>
  deployModal.value ? deployTakenServerIds(deployModal.value.platformId, deployModal.value.editId) : [],
);

async function loadDeployServerList() {
  try {
    deployServers.value = (await api.getAdminServers()) ?? [];
  } catch {
    deployServers.value = [];
  }
}

async function openDeploy(t: FRPTunnelItem | null = null) {
  if (!deployServers.value.length) await loadDeployServerList();
  const platformId = t?.platform_id || platforms.value[0]?.id || 0;
  deployModal.value = {
    editId: 0,
    platformId,
    serverId: '',
    tunnelIds: t ? [t.id] : [],
    installDocker: false,
    docker: null,
    probing: false,
    busy: false,
    elapsed: 0,
    error: '',
  };
}

// 调整已有托管的隧道集合：平台/节点锁死（换机器走「移除」再部署，避免留下孤儿容器）
function openDeployEdit(d: FRPDeployItem) {
  deployModal.value = {
    editId: d.id,
    platformId: d.platform_id,
    serverId: String(d.server_id),
    tunnelIds: [...(d.tunnel_ids || [])],
    installDocker: false,
    docker: { present: true, version: d.docker_ver, daemon_ok: true, err: '' },
    probing: false,
    busy: false,
    elapsed: 0,
    error: '',
  };
}

function onDeployPlatformChange() {
  const m = deployModal.value;
  if (!m) return;
  m.tunnelIds = [];
  m.docker = null;
}

async function probeDocker() {
  const m = deployModal.value;
  if (!m) return;
  if (!m.serverId) {
    m.error = '请先选择节点';
    return;
  }
  m.probing = true;
  m.error = '';
  try {
    const r = await api.serverDocker(Number(m.serverId), false);
    m.docker = r.docker ?? { present: false, version: '', daemon_ok: false, err: r.error || '检测失败' };
  } catch (e) {
    m.docker = { present: false, version: '', daemon_ok: false, err: e instanceof Error ? e.message : '检测失败' };
  } finally {
    m.probing = false;
  }
}

async function submitDeploy() {
  const m = deployModal.value;
  if (!m) return;
  if (!m.serverId) {
    m.error = '请选择节点';
    return;
  }
  if (!m.tunnelIds.length) {
    m.error = '请至少勾选一条隧道';
    return;
  }
  m.busy = true;
  m.error = '';
  // 装 Docker + 拉镜像可能好几分钟，走动的秒数让用户知道没卡死
  const t0 = Date.now();
  m.elapsed = 0;
  const tick = window.setInterval(() => {
    if (deployModal.value) deployModal.value.elapsed = Math.round((Date.now() - t0) / 1000);
  }, 1000);
  try {
    await api.createFRPDeployment({
      platform_id: m.platformId,
      server_id: Number(m.serverId),
      tunnel_ids: m.tunnelIds,
      install_docker: m.installDocker,
    });
    toast(m.editId ? '已重新同步托管客户端' : '客户端已在节点上部署');
    closeModal();
    await loadDeployments(true);
  } catch (e) {
    m.error = e instanceof Error ? e.message : '部署失败';
  } finally {
    window.clearInterval(tick);
    if (deployModal.value) deployModal.value.busy = false;
  }
}

async function runDeployOp(d: FRPDeployItem, fn: () => Promise<void>, okText?: string) {
  deployOpsId.value = d.id;
  try {
    await fn();
    if (okText) toast(okText);
  } catch (e) {
    toast(e instanceof Error ? e.message : '操作失败', true);
  } finally {
    deployOpsId.value = 0;
  }
}

function deploySync(d: FRPDeployItem) {
  return runDeployOp(d, async () => {
    const r = await api.syncFRPDeployment(d.id);
    await loadDeployments(true);
    toast(r?.log_tail ? '已同步（容器有输出，建议看日志）' : '配置已同步');
  });
}

function deployAction(d: FRPDeployItem, action: 'restart' | 'stop' | 'start') {
  return runDeployOp(
    d,
    async () => {
      await api.actionFRPDeployment(d.id, action);
      await loadDeployments(true);
    },
    action === 'restart' ? '容器已重启' : action === 'stop' ? '容器已停止' : '容器已启动',
  );
}

async function openDeployLogs(d: FRPDeployItem) {
  deployLogs.value = { id: d.id, container: d.container, loading: true, text: '', error: '' };
  try {
    const r = await api.getFRPDeployLogs(d.id, 200);
    if (deployLogs.value?.id === d.id) {
      deployLogs.value.container = r.container || d.container;
      deployLogs.value.text = r.logs || '';
      deployLogs.value.loading = false;
    }
  } catch (e) {
    if (deployLogs.value?.id === d.id) {
      deployLogs.value.loading = false;
      deployLogs.value.error = e instanceof Error ? e.message : '读取日志失败';
    }
  }
}

async function reloadDeployLogs() {
  const id = deployLogs.value?.id;
  if (!id) return;
  deployLogs.value = { ...deployLogs.value!, loading: true, error: '' };
  try {
    const r = await api.getFRPDeployLogs(id, 200);
    if (deployLogs.value) {
      deployLogs.value.text = r.logs || '';
      deployLogs.value.loading = false;
    }
  } catch (e) {
    if (deployLogs.value) {
      deployLogs.value.loading = false;
      deployLogs.value.error = e instanceof Error ? e.message : '读取日志失败';
    }
  }
}

function askDeleteDeploy(d: FRPDeployItem) {
  confirmState.value = {
    title: '移除托管',
    message: `确认移除「${d.server_name}」上的 ${kindLabel[d.platform_kind] || d.platform_kind} 客户端？`
      + '将删除节点上的容器与配置（含平台令牌）；若只想停用，可改用「⋯ → 停止」。',
    confirmText: '移除',
    danger: true,
    busy: false,
    run: async () => {
      await api.deleteFRPDeployment(d.id, false);
      await loadDeployments();
      toast('托管已移除');
    },
  };
}

// 释放占用：让面板托管的客户端停下，把隧道让给别的机器/官方客户端。
// 单独成动作是因为「面板占着隧道但用户不知道」会让平台侧后续登录一直 proxy conflict。
function askReleaseTunnel(t: FRPTunnelItem) {
  const d = deployByTunnel.value[t.id];
  if (!d) return;
  confirmState.value = {
    title: '释放隧道占用',
    message: `确认让「${d.server_name}」上的面板客户端停止占用隧道「${t.name}」？`
      + '将停止该节点上的 frpc 容器（配置与记录保留，随时可再启动）；之后这条隧道可由其它机器或官方客户端接管。',
    confirmText: '停止占用',
    danger: false,
    busy: false,
    run: async () => {
      await api.actionFRPDeployment(d.id, 'stop');
      await loadDeployments(true);
      toast('已释放：该隧道的客户端已停止');
    },
  };
}

// ---------- 二级域名 ----------
const subs = ref<{ loading: boolean; list: FRPSubdomainItem[]; domains: { domain: string }[]; error: string }>({
  loading: false,
  list: [],
  domains: [],
  error: '',
});
const subModal = ref<{
  platformId: number;
  mode: 'create' | 'edit';
  domain: string;
  record: string;
  type: string;
  target: string;
  ttl: string;
  remarks: string;
  busy: boolean;
  error: string;
} | null>(null);

function openSubCreate() {
  subModal.value = {
    platformId: chmlPlatform.value?.id || 0,
    mode: 'create',
    domain: subs.value.domains.map((d) => d.domain)[0] || '',
    record: '',
    type: 'A',
    target: '',
    ttl: '10分钟',
    remarks: '',
    busy: false,
    error: '',
  };
}
function openSubEdit(sd: FRPSubdomainItem) {
  subModal.value = {
    platformId: chmlPlatform.value?.id || 0,
    mode: 'edit',
    domain: sd.domain,
    record: sd.record,
    type: sd.type,
    target: sd.target,
    ttl: sd.ttl,
    remarks: sd.remarks || '',
    busy: false,
    error: '',
  };
}
async function submitSub() {
  const m = subModal.value;
  if (!m) return;
  if (!m.domain || !m.record.trim() || !m.target.trim()) {
    m.error = '主域名、主机记录、目标地址均为必填';
    return;
  }
  m.busy = true;
  m.error = '';
  const payload = {
    domain: m.domain,
    record: m.record.trim(),
    type: m.type,
    target: m.target.trim(),
    ttl: m.ttl,
    remarks: m.remarks || '',
  };
  try {
    if (m.mode === 'create') await api.createFRPSubdomain(m.platformId, payload);
    else await api.updateFRPSubdomain(m.platformId, payload);
    closeModal();
    await loadAllSubdomains();
    toast(m.mode === 'create' ? '解析已创建' : '解析已更新');
  } catch (e) {
    m.error = e instanceof Error ? e.message : '提交失败';
  } finally {
    if (subModal.value) subModal.value.busy = false;
  }
}

// ---------- 流量历史（多平台同图） ----------
const TREND_COLORS: Record<string, string> = { natfrp: '#0ea5e9', chmlfrp: '#8b5cf6', cloudflared: '#f97316' };
const trendLoading = ref(false);
const trendSeries = ref<{ name: string; color: string; data: (number | null)[] }[]>([]);
const trendLabels = ref<string[]>([]);

async function loadTrend() {
  if (!platforms.value.length) return;
  trendLoading.value = true;
  try {
    const results = await Promise.all(
      platforms.value.map(async (p) => {
        try {
          const r = await api.getFRPFlow(p.id, 'day');
          return { p, points: r.points || [] };
        } catch {
          return { p, points: [] };
        }
      }),
    );
    const withData = results.filter((r) => r.points.length);
    const base = withData.reduce<{ label: string }[]>((best, r) => (r.points.length > best.length ? r.points : best), []);
    trendLabels.value = base.map((x) => x.label);
    trendSeries.value = withData.map((r) => ({
      name: r.p.name,
      color: TREND_COLORS[r.p.kind] || '#0ea5e9',
      data: trendLabels.value.map((l) => Number(r.points.find((x) => x.label === l)?.used) || 0),
    }));
  } finally {
    trendLoading.value = false;
  }
}

onMounted(async () => {
  loading.value = true;
  try {
    await loadOverview();
    await Promise.all([loadDetails(), loadDeployments()]);
    await Promise.all([loadNodes(), loadAllSubdomains()]);
    void loadTrend();
    idleTimer = window.setInterval(idleReload, 60000);
  } finally {
    loading.value = false;
  }
});

onBeforeUnmount(() => {
  window.clearInterval(idleTimer);
  if (pollTimer) window.clearInterval(pollTimer);
  clearTimeout(tipTimer);
});
</script>
