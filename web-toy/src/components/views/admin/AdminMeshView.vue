<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-3 mb-2">
      <div class="space-y-1.5">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h2 class="text-xl font-black text-toy-black">WG 组网管理</h2>
          <span class="toy-badge bg-toy-yellow text-[11px] px-2.5 py-0.5">A/B 主备中心</span>
        </div>
        <p class="text-xs font-bold text-zinc-500">WireGuard 全自动星型组网 · 流量额度轮换 · 跨设备凭证分发</p>
      </div>

      <div class="flex items-center gap-2">
        <button 
          @click="handlePatrol"
          class="toy-btn px-3 py-1.5 bg-white hover:bg-toy-bg text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
          :disabled="isPatrolling || overview.running_task"
        >
          <span>{{ isPatrolling ? '⏳' : '🔍' }}</span>
          <span>{{ isPatrolling ? '正在巡检全网...' : '全网巡检' }}</span>
        </button>
        <button 
          @click="openWizard"
          class="toy-btn px-3 py-1.5 bg-white hover:bg-toy-bg text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
          :disabled="overview.running_task"
        >
          <span>🧭</span>
          <span>{{ overview.network ? '重配组网' : '组网向导' }}</span>
        </button>
        <details class="relative">
          <summary class="toy-btn px-3 py-1.5 bg-white hover:bg-toy-bg text-xs font-black rounded-xl cursor-pointer list-none flex items-center gap-1.5">
            <span>⋯</span><span>更多</span>
          </summary>
          <div class="absolute right-0 z-40 mt-1 w-52 bg-white border-2 border-toy-black rounded-xl shadow-toy-md p-1.5 space-y-0.5">
            <button @click="openImport" :disabled="overview.running_task" class="w-full text-left px-2 py-1.5 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer disabled:opacity-50">导入现有网络</button>
            <button @click="openTakeover" :disabled="overview.running_task" class="w-full text-left px-2 py-1.5 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer disabled:opacity-50">新机接管现役中心</button>
            <button @click="downloadCreds" class="w-full text-left px-2 py-1.5 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer">下载使用端凭证（zip）</button>
            <button @click="toggleAssetsPanel" class="w-full text-left px-2 py-1.5 rounded text-[11px] font-bold hover:bg-toy-bg cursor-pointer">
              {{ assetsPanelOpen ? '收起资产中转' : '展开资产中转' }}
            </button>
          </div>
        </details>
        <button 
          @click="openAddDeviceModal"
          class="toy-btn px-3.5 py-1.5 bg-toy-yellow text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
          :disabled="overview.running_task"
        >
          <span>➕</span>
          <span>接入外部设备</span>
        </button>
      </div>
    </div>

    <!-- 错误/提示条（真实操作反馈） -->
    <div v-if="errorMsg" class="toy-card p-3 bg-red-50 border-2.5 border-red-400 flex items-center justify-between gap-3">
      <span class="text-xs font-bold text-red-700">⚠ {{ errorMsg }}</span>
      <button @click="errorMsg = ''" class="text-xs font-black cursor-pointer">✕</button>
    </div>
    <div v-if="okMsg" class="toy-card p-3 bg-toy-mint/40 border-2.5 border-toy-black flex items-center justify-between gap-3">
      <span class="text-xs font-bold text-toy-black">✔ {{ okMsg }}</span>
      <button @click="okMsg = ''" class="text-xs font-black cursor-pointer">✕</button>
    </div>
    <div v-if="overview.running_task && !taskModal" class="toy-card p-3 bg-toy-yellow/30 border-2.5 border-toy-black flex items-center gap-2">
      <span class="w-2 h-2 rounded-full bg-toy-black animate-ping"></span>
      <span class="text-xs font-black text-toy-black">后台组网任务执行中，页面每 5 秒自动刷新进度…</span>
    </div>

    <!-- 0. 未初始化引导（对齐 classic：导入现有网络 / 全新组网入口） -->
    <div v-if="!isLoading && !overview.network" class="toy-card p-8 bg-white text-center space-y-3">
      <span class="text-4xl">🛰️</span>
      <h3 class="text-base font-black text-toy-black">尚未初始化 WG 组网</h3>
      <p class="text-xs font-bold text-toy-muted max-w-xl mx-auto">
        已有现网（如 wg1/wg2 星型组网）可用「导入现有网络」直接收编；
        也可以先在节点管理录入一台公网服务器（要能 SSH 登录、云安全组放行 UDP 51820 入方向），
        再用「组网向导」从零组一张新网。
      </p>
      <div class="flex items-center justify-center gap-2 flex-wrap pt-1">
        <button @click="openImport" class="toy-btn px-3.5 py-1.5 bg-white text-xs font-black rounded-xl cursor-pointer">导入现有网络</button>
        <button @click="openWizard" class="toy-btn px-3.5 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer">组网向导</button>
      </div>
      <p v-if="selfServer && !selfServer.ssh_ready" class="text-[11px] font-bold text-toy-orange">
        ⚠ 本机尚未录宿主 SSH：向导里会不可选，先到「节点管理」补录。
      </p>
    </div>

    <template v-if="overview.network">
      <!-- 1. 中心槽位卡片：现役与所有备援共用同一张模板（同一份字段，避免某台备援少显示一截） -->
      <div class="grid gap-4 items-start grid-cols-[repeat(auto-fit,minmax(340px,1fr))]">
        <div
          v-for="h in orderedHubs"
          :key="`hub-${h.server_id}`"
          class="toy-card p-5 bg-white border-2.5 border-toy-black relative overflow-hidden cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-toy-black transition-all hover:-translate-y-0.5"
          role="button"
          tabindex="0"
          :aria-label="`管理中心节点 ${h.name}`"
          @click="openHub(h)"
          @keydown.enter="openHub(h)"
          @keydown.space.prevent="openHub(h)"
        >
          <!-- 角色标头栏 -->
          <div class="flex items-center justify-between gap-2 border-b-2 border-toy-black/10 pb-3">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-xl">{{ h.is_active ? '👑' : '🛡️' }}</span>
              <span class="font-black text-sm text-toy-black">
                {{ h.is_active ? '现役主中心' : '备援中心' }}
              </span>
              <span
                class="px-2 py-0.5 rounded text-[11px] font-black border border-toy-black"
                :class="h.is_active ? 'bg-toy-mint text-toy-black' : 'bg-zinc-100 text-zinc-600'"
              >
                {{ h.is_active ? '承载全网流量' : '待命就绪' }}
              </span>
              <span class="px-2 py-0.5 rounded text-[11px] font-black border border-toy-black" :class="hubStatusTag(h).cls">
                {{ hubStatusTag(h).text }}
              </span>
            </div>
            <div
              class="w-9 h-9 rounded-xl border-2 border-toy-black flex items-center justify-center font-black text-xs shadow-toy-xs shrink-0"
              :class="h.is_active ? 'bg-toy-yellow text-toy-black' : 'bg-gray-100 text-zinc-600'"
            >
              {{ h.is_active ? 'HUB' : 'BAK' }}
            </div>
          </div>

          <!-- 节点核心信息与监听端点 -->
          <div class="mt-3.5 space-y-1.5">
            <div class="text-base font-black text-toy-black font-sans break-all tracking-tight">
              {{ h.name }}
            </div>
            <div class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-toy-bg border border-toy-black/20 text-xs font-mono font-bold text-zinc-700">
              <span class="text-zinc-500">端点</span>
              <span class="text-toy-black font-extrabold">{{ h.endpoint || '端点未知' }}</span>
              <span class="text-zinc-300">|</span>
              <span class="text-zinc-500">UDP {{ h.listen_port }}</span>
            </div>
          </div>

          <!-- 流量计量与计费口径 -->
          <div class="mt-4 p-3.5 bg-toy-bg rounded-xl border-2 border-toy-black space-y-2.5">
            <div class="flex items-center justify-between flex-wrap gap-2">
              <div>
                <span class="text-zinc-500 block text-[11px] font-black uppercase">本月出向（计费）</span>
                <div class="text-lg font-black font-mono text-toy-black mt-0.5">
                  {{ formatBytes(h.month_billed) }}<template v-if="h.quota_gb"> <span class="text-xs text-zinc-500 font-bold">/ {{ h.quota_gb }} GB</span></template>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <button
                  @click.stop="probeHub(h.server_id)"
                  class="px-2.5 py-1 bg-white border-2 border-toy-black rounded-lg text-xs font-black hover:bg-toy-yellow cursor-pointer shadow-toy-xs"
                  title="探测与中心的网络往返延迟"
                >
                  ⚡ {{ hubLatency[h.server_id] !== undefined ? `${hubLatency[h.server_id]}ms` : '测速' }}
                </button>
                <button
                  v-if="!h.is_active"
                  @click.stop="handleSwitchHub(h.server_id)"
                  class="toy-btn px-3 py-1 bg-toy-yellow text-xs font-black rounded-lg cursor-pointer"
                  :disabled="overview.running_task"
                >
                  切换为现役
                </button>
              </div>
            </div>

            <!-- 额度进度条 -->
            <div v-if="h.quota_gb" class="h-2 rounded-full border-1.5 border-toy-black bg-white overflow-hidden">
              <div
                class="h-full transition-all"
                :class="quotaPct(h) >= 100 ? 'bg-toy-pink' : quotaPct(h) > 80 ? 'bg-toy-orange' : 'bg-toy-mint'"
                :style="{ width: `${Math.min(100, quotaPct(h))}%` }"
              ></div>
            </div>

            <div class="text-[11px] font-bold text-zinc-600 font-mono flex items-center justify-between">
              <span>双向合计 {{ formatBytes(h.month_rx + h.month_tx) }}</span>
              <span class="text-zinc-400">入向免计费</span>
            </div>
          </div>

          <!-- 异常信息警告 -->
          <div v-if="h.last_error" class="mt-3 p-2.5 bg-red-50 border-2 border-red-400 rounded-lg text-xs font-bold text-red-700">
            ⚠ {{ h.last_error }}
          </div>

          <!-- 底部密钥状态与凭证入口 -->
          <div class="mt-3.5 pt-2.5 border-t border-toy-black/10 flex items-center justify-between flex-wrap gap-2 text-xs font-bold">
            <span
              class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md border border-toy-black/30 font-sans text-[11px]"
              :class="h.has_keys ? 'bg-emerald-50 text-emerald-800 border-emerald-300' : 'bg-rose-50 text-rose-800 border-rose-300'"
            >
              <span>🔑</span>
              <span>{{ h.has_keys
                ? (h.is_active ? '面板已托管中心密钥' : '密钥与路由配置就绪')
                : '未托管私钥（只读不可改）' }}</span>
            </span>
            <span class="text-xs font-black text-toy-black flex items-center gap-1">
              <span>管理凭证</span>
              <span class="text-sm">›</span>
            </span>
          </div>

          <!-- 现役提示 -->
          <div v-if="h.is_active" class="mt-2 text-[11px] font-bold text-zinc-500">
            {{ standbyHubs.length ? '💡 流量超限或维护时可一键平滑轮换至备援' : '💡 当前无备援：网外节点点击「配置为备援」可组建高可用容灾' }}
          </div>
        </div>

        <!-- 已退役中心 -->
        <div v-if="overview.retired_hubs.length" class="toy-card p-4 bg-white border-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="font-black text-sm text-toy-black">已退役中心</span>
            <span class="toy-badge bg-gray-100 text-[10px]">身份已迁走</span>
          </div>
          <div v-for="r in overview.retired_hubs" :key="`ret-${r.server_id}`" class="mt-2 flex items-center justify-between gap-2 text-xs font-bold text-toy-muted">
            <span class="font-mono">{{ r.name }}</span>
            <button
              class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-red-50 text-red-600 cursor-pointer"
              :disabled="overview.running_task"
              @click="cleanupRetired(r)"
            >清理记录</button>
          </div>
          <p class="mt-2 text-[10px] font-bold text-toy-muted">机器上的配置在退役时已按时间戳备份（.removed.*）</p>
        </div>

        <!-- 网外未纳管节点：配置为备援 / 只读登记 / 纳管本机 -->
        <div
          v-for="s in unmanagedServers"
          :key="`idle-${s.id}`"
          class="toy-card p-4 bg-white border-2.5 border-dashed border-toy-black/70"
        >
          <div class="flex items-center gap-2 flex-wrap">
            <span class="font-black text-sm text-toy-black">{{ s.name }}</span>
            <span class="toy-badge bg-gray-100 text-[10px]">未纳管</span>
            <span v-if="s.is_self" class="toy-badge bg-toy-cyan/50 text-[10px]">本机</span>
          </div>
          <p class="mt-1 text-[11px] font-bold text-toy-muted">
            {{ s.is_self
              ? '本机（面板宿主）· 采集走本地进程，组网需经宿主 SSH'
              : '不在本网 · 一键配置会生成密钥、下发全部成员并拉起接口' }}
          </p>
          <div class="mt-2.5 flex items-center gap-2 flex-wrap">
            <template v-if="s.is_self">
              <button
                class="toy-btn px-3 py-1 bg-toy-yellow text-[11px] font-black rounded-lg cursor-pointer"
                :disabled="busyServerId === s.id || overview.running_task || !activeHub || !s.ssh_ready"
                @click="adoptServer(s)"
              >{{ busyServerId === s.id ? '纳管中…' : '纳管本机入网' }}</button>
            </template>
            <template v-else>
              <button
                class="toy-btn px-3 py-1 bg-toy-yellow text-[11px] font-black rounded-lg cursor-pointer"
                :disabled="busyServerId === s.id || overview.running_task || !s.ssh_ready"
                @click="provisionServer(s)"
              >{{ busyServerId === s.id ? '提交中…' : '配置为备援' }}</button>
              <button
                class="px-3 py-1 bg-white border-2 border-toy-black text-[11px] font-black rounded-lg hover:bg-toy-bg cursor-pointer disabled:opacity-50"
                title="机器上已有手工配好的 WG 配置，只读取登记、不写目标机"
                :disabled="busyServerId === s.id || overview.running_task || !activeHub"
                @click="registerStandby(s)"
              >只读登记</button>
            </template>
          </div>
          <p v-if="!s.ssh_ready" class="mt-2 text-[10px] font-bold text-toy-orange">
            ⚠ {{ s.is_self ? '先在「节点管理」录宿主 SSH（容器可达地址）后即可纳管' : '先在「节点管理」录入 SSH 凭据（面板代管密钥会自动装机）后即可一键配置' }}
          </p>
        </div>
      </div>

      <!-- 2. 网段配置与状态条 -->
      <div class="toy-card p-3.5 bg-toy-bg border-2 border-toy-black flex items-center justify-between flex-wrap gap-2 text-xs font-bold font-mono">
        <div class="flex items-center gap-3 flex-wrap">
          <span class="flex items-center gap-1.5 text-toy-black">
            <span class="w-2 h-2 rounded-full bg-toy-mint"></span>
            虚拟专用网段: <span class="bg-white px-2 py-0.5 rounded border border-toy-black font-black">{{ overview.network.subnet }}</span>
          </span>
          <span class="hidden sm:inline text-toy-muted">|</span>
          <span class="text-toy-muted">hub 虚拟 IP <b class="text-toy-black">{{ overview.network.hub_ip }}</b></span>
          <span class="text-toy-muted">接口 <b class="text-toy-black">{{ overview.network.iface }}</b></span>
          <span class="text-toy-muted">保活 {{ overview.network.keepalive }}s · MTU {{ overview.network.mtu }}</span>
          <span v-if="activeHub" class="text-toy-muted">现役 <b class="text-toy-black">{{ activeHub.name }}</b></span>
          <span v-if="overview.running_task" class="toy-badge bg-toy-orange text-[10px]">任务执行中…</span>
        </div>
        <div class="text-toy-muted">
          在网节点: <span class="text-toy-black font-extrabold">{{ serverPeers.length }}</span> 台 · 外部终端: <span class="text-toy-black font-extrabold">{{ devicePeers.length }}</span> 台
        </div>
      </div>

      <!-- 3. 星型拓扑简图 (按真实成员渲染) -->
      <div class="toy-card p-5 bg-white space-y-3.5">
        <div class="flex items-center justify-between flex-wrap gap-2">
          <div class="flex items-center gap-2">
            <h3 class="text-sm font-black text-toy-black">全网星型拓扑架构</h3>
            <span class="px-2 py-0.5 rounded bg-toy-yellow text-[10px] font-black border border-toy-black">实时互联</span>
          </div>
          <span class="text-xs font-mono text-zinc-500 font-bold">中心汇聚 · 零外网穿透暴露</span>
        </div>
        <div class="w-full bg-toy-bg rounded-xl border-2.5 border-toy-black p-4 overflow-x-auto shadow-inner">
          <svg class="w-full min-w-[580px] h-64 select-none" viewBox="0 0 720 270">
            <defs>
              <pattern id="topoGrid" width="20" height="20" patternUnits="userSpaceOnUse">
                <circle cx="2" cy="2" r="1" fill="#121212" fill-opacity="0.08" />
              </pattern>
            </defs>
            <rect width="720" height="270" fill="url(#topoGrid)" rx="10" />

            <!-- 连线（从中心到各个成员卡片中心） -->
            <line
              v-for="n in topoNodes"
              :key="`l-${n.key}`"
              x1="360" y1="135" :x2="n.x" :y2="n.y"
              stroke="#121212" stroke-width="2.2" stroke-dasharray="5 4"
            />

            <!-- 中心 Hub 卡片 -->
            <g class="cursor-pointer" :title="`中心节点: ${activeHub?.name || '未配置'}`">
              <!-- 投影 -->
              <rect x="293.5" y="109.5" width="140" height="58" rx="14" fill="#121212" />
              <!-- 卡片主体 -->
              <rect
                x="290" y="106" width="140" height="58" rx="14"
                fill="#FFE047" stroke="#121212" stroke-width="2.5"
              />
              <text x="360" y="130" font-size="12" font-weight="900" text-anchor="middle" fill="#121212">
                👑 {{ shortName(activeHub?.name || '未初始化') }}
              </text>
              <text x="360" y="150" font-size="10.5" font-weight="800" font-family="monospace" text-anchor="middle" fill="#121212">
                {{ overview.network.hub_ip }}
              </text>
            </g>

            <!-- 成员与设备卡片 (避免圆圈挤字与溢出) -->
            <g v-for="n in topoNodes" :key="n.key" class="transition-transform">
              <!-- 投影 -->
              <rect :x="n.x - 54 + 2.5" :y="n.y - 22 + 2.5" width="108" height="44" rx="10" fill="#121212" />
              <!-- 卡片主体 -->
              <rect
                :x="n.x - 54" :y="n.y - 22" width="108" height="44" rx="10"
                :fill="n.kind === 'device' ? '#E6FAF2' : (n.role === 'standby' ? '#F1F5F9' : '#FFFFFF')"
                stroke="#121212" stroke-width="2.2"
              />
              <!-- 节点名称 -->
              <text :x="n.x" :y="n.y - 4" font-size="10.5" font-weight="900" text-anchor="middle" fill="#121212">
                {{ (n.kind === 'device' ? '📱 ' : '🖥️ ') + shortName(n.name) }}
              </text>
              <!-- 虚拟 IP -->
              <text
                :x="n.x" :y="n.y + 11" font-size="9.5" font-weight="800" font-family="monospace" text-anchor="middle"
                :fill="n.kind === 'device' ? '#047857' : '#4B5563'"
              >
                {{ n.wg_ip }}
              </text>
            </g>

            <text v-if="!topoNodes.length" x="360" y="225" font-size="11" font-weight="800" text-anchor="middle" fill="#71717A">
              暂无已连接成员
            </text>
          </svg>
        </div>
      </div>

      <!-- 4. 成员节点与终端设备分表管理 -->
      <div class="space-y-4">
        <!-- 4.1 纳管服务器成员 -->
        <div class="toy-card p-5 bg-white space-y-3">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-black text-toy-black flex items-center gap-2">
              <span>纳管服务器节点 ({{ serverPeers.length }})</span>
              <span class="toy-badge bg-toy-yellow/50 text-[10px]">SSH 自动下发</span>
            </h3>
            <span class="text-xs text-toy-muted font-bold font-mono">Keepalive: {{ overview.network.keepalive }}s</span>
          </div>

          <div class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead>
                <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                  <th class="pb-2">节点标识</th>
                  <th class="pb-2">类型</th>
                  <th class="pb-2">网内虚拟 IP</th>
                  <th class="pb-2">状态</th>
                  <th class="pb-2">最近握手</th>
                  <th class="pb-2">收 / 发</th>
                  <th class="pb-2 text-right">操作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-toy-gray font-mono">
                <tr v-for="peer in serverPeers" :key="peer.id" class="hover:bg-toy-bg/50 transition-colors">
                  <td class="py-3 font-bold font-sans text-toy-black">
                    <div class="flex items-center gap-1.5">
                      <span v-if="peer.server_id && peer.server_id === overview.network.active_hub_server_id" class="text-xs">👑</span>
                      <span>{{ peer.name }}</span>
                      <span v-if="peer.last_error" class="text-toy-pink cursor-help" :title="peer.last_error">⚠</span>
                    </div>
                  </td>
                  <td class="py-3 text-toy-muted font-sans">{{ peer.kind === 'device' ? '设备' : '节点' }}</td>
                  <td class="py-3 font-extrabold text-toy-black">{{ peer.wg_ip }}</td>
                  <td class="py-3">
                    <span class="toy-badge text-[10px]" :class="peerStatusTag(peer).cls">{{ peerStatusTag(peer).text }}</span>
                  </td>
                  <td class="py-3 text-toy-black font-sans font-bold">{{ handshakeAgo(peer.last_handshake) }}</td>
                  <td class="py-3 text-toy-muted">{{ formatBytes(peer.rx_bytes) }} / {{ formatBytes(peer.tx_bytes) }}</td>
                  <td class="py-3 text-right space-x-1.5 font-sans">
                    <button
                      class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-cyan transition-colors cursor-pointer disabled:opacity-50"
                      :disabled="verifyingId === peer.id || overview.running_task"
                      @click="verifyPeer(peer)"
                    >{{ verifyingId === peer.id ? '验证中…' : '验证' }}</button>
                    <button
                      class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-red-50 text-red-600 transition-colors cursor-pointer"
                      :disabled="overview.running_task"
                      @click="handleDeletePeer(peer)"
                    >移出</button>
                  </td>
                </tr>
                <tr v-if="!serverPeers.length">
                  <td colspan="7" class="py-6 text-center text-toy-muted font-bold font-sans">暂无服务器成员</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- 4.2 外部客户端设备 (手机/电脑等) -->
        <div class="toy-card p-5 bg-white space-y-3">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-black text-toy-black flex items-center gap-2">
              <span>外部接入终端 ({{ devicePeers.length }})</span>
              <span class="toy-badge bg-toy-mint text-[10px]">WireGuard Client</span>
            </h3>
            <button 
              @click="openAddDeviceModal"
              class="toy-btn px-2.5 py-1 bg-white hover:bg-toy-yellow text-[11px] font-black rounded-lg cursor-pointer"
              :disabled="overview.running_task"
            >
              ➕ 生成新设备凭据
            </button>
          </div>

          <div class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead>
                <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                  <th class="pb-2">设备名称</th>
                  <th class="pb-2">类型</th>
                  <th class="pb-2">分配 IP</th>
                  <th class="pb-2">最近通信</th>
                  <th class="pb-2">收 / 发</th>
                  <th class="pb-2">状态</th>
                  <th class="pb-2 text-right">操作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-toy-gray font-mono">
                <tr v-for="dev in devicePeers" :key="dev.id" class="hover:bg-toy-bg/50 transition-colors">
                  <td class="py-3 font-bold font-sans text-toy-black flex items-center gap-2">
                    <span>📱</span>
                    <span>{{ dev.name }}</span>
                    <span v-if="dev.last_error" class="text-toy-pink cursor-help" :title="dev.last_error">⚠</span>
                  </td>
                  <td class="py-3 text-toy-muted font-sans">{{ dev.kind === 'device' ? '设备' : '节点' }}</td>
                  <td class="py-3 font-extrabold text-toy-black">{{ dev.wg_ip }}</td>
                  <td class="py-3 font-sans font-bold text-toy-muted">{{ handshakeAgo(dev.last_handshake) }}</td>
                  <td class="py-3 text-toy-muted">{{ formatBytes(dev.rx_bytes) }} / {{ formatBytes(dev.tx_bytes) }}</td>
                  <td class="py-3">
                    <span class="toy-badge text-[10px]" :class="peerStatusTag(dev).cls">{{ peerStatusTag(dev).text }}</span>
                  </td>
                  <td class="py-3 text-right space-x-1.5 font-sans">
                    <button 
                      class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-toy-yellow hover:bg-yellow-300 transition-colors cursor-pointer"
                      :disabled="!dev.can_export"
                      :title="dev.can_export ? '查看配置与二维码' : '导入成员不持私钥，先「重新签发」后再导出'"
                      @click="viewDeviceConfig(dev)"
                    >
                      配置 / 扫码
                    </button>
                    <button 
                      class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg transition-colors cursor-pointer disabled:opacity-50"
                      :disabled="overview.running_task"
                      @click="renamePeer(dev)"
                    >重命名</button>
                    <button 
                      class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-cyan transition-colors cursor-pointer disabled:opacity-50"
                      :disabled="overview.running_task"
                      @click="reissuePeer(dev)"
                    >重新签发</button>
                    <button 
                      class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-red-50 text-red-600 transition-colors cursor-pointer"
                      :disabled="overview.running_task"
                      @click="handleDeletePeer(dev)"
                    >
                      移除
                    </button>
                  </td>
                </tr>
                <tr v-if="!devicePeers.length">
                  <td colspan="7" class="py-6 text-center text-toy-muted font-bold font-sans">暂无外部终端，点右上「生成新设备凭据」开始</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </template>

    <!-- 弹窗 0: 后台任务进度（切换中心 / 配置备援等） -->
    <div v-if="taskModal" class="fixed inset-0 z-[55] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-5 max-h-[85vh] overflow-y-auto" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black flex items-center gap-2">
            <span v-if="taskModalDone">✔</span><span v-else class="w-2 h-2 rounded-full bg-toy-black animate-ping"></span>
            组网任务 #{{ taskModal.id }}
          </h3>
          <button
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
            @click="closeTaskModal"
          >✕</button>
        </div>
        <p class="mt-2 text-[11px] font-bold text-toy-muted">
          {{ taskModalStatusText }}
        </p>
        <div class="mt-3 space-y-2">
          <div
            v-for="st in taskModal.steps"
            :key="st.id"
            class="p-2.5 rounded-lg border-2 border-toy-black bg-toy-bg/60 space-y-0.5"
          >
            <div class="flex items-center justify-between gap-2 text-xs font-black text-toy-black">
              <span>{{ st.seq + 1 }}. {{ st.title }}</span>
              <span class="toy-badge text-[10px]" :class="stepStatusTag(st.status).cls">{{ stepStatusTag(st.status).text }}</span>
            </div>
            <p v-if="st.log" class="text-[10px] font-mono text-toy-muted break-all">{{ st.log }}</p>
          </div>
          <p v-if="!taskModal.steps.length" class="text-xs font-bold text-toy-muted">任务已提交，等待步骤回报…</p>
        </div>
      </div>
    </div>

    <!-- 弹窗 1: 查看配置 & 扫码二维码弹窗 -->
    <div v-if="selectedDevice" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div 
        class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 relative overflow-hidden"
        :style="{ boxShadow: '6px 6px 0px #121212' }"
      >
        <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="text-xl">📱</span>
            <div>
              <h3 class="text-base font-black text-toy-black">{{ selectedDevice.name }}</h3>
              <p class="text-[10px] font-bold text-toy-muted">WireGuard 客户端导入凭证 · {{ selectedDevice.wg_ip }}</p>
            </div>
          </div>
          <button 
            @click="selectedDevice = null"
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
          >
            ✕
          </button>
        </div>

        <div class="mt-4 space-y-4">
          <!-- 二维码显示区 -->
          <div class="flex flex-col items-center justify-center p-4 bg-toy-bg rounded-xl border-2 border-toy-black">
            <canvas ref="qrCanvasRef" class="w-40 h-40 border-2 border-toy-black rounded-lg shadow-toy-sm"></canvas>
            <p class="mt-2 text-[11px] font-bold text-toy-muted">手机 WireGuard App 直接扫描上方二维码即可添加</p>
          </div>

          <!-- 文本配置预览 -->
          <div>
            <div class="flex items-center justify-between text-xs font-black text-toy-black mb-1">
              <span>wg0.conf 配置文本</span>
              <button @click="copyConfText" class="text-sky-700 hover:underline cursor-pointer">
                {{ isCopied ? '已复制 ✔' : '复制全文' }}
              </button>
            </div>
            <textarea 
              readonly
              :value="currentConfText"
              rows="5"
              class="w-full p-2.5 bg-toy-bg border-2 border-toy-black rounded-lg text-[10px] font-mono focus:outline-none"
            ></textarea>
          </div>

          <div class="flex items-center justify-end gap-2 pt-1">
            <button 
              @click="downloadConfFile"
              class="toy-btn px-4 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer"
            >
              📥 下载 .conf 文件
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 弹窗 2: 新增设备弹窗 -->
    <div v-if="isAddDeviceModalOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div 
        class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-6 relative overflow-hidden"
        :style="{ boxShadow: '6px 6px 0px #121212' }"
      >
        <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="text-xl">➕</span>
            <div>
              <h3 class="text-base font-black text-toy-black">接入新终端设备</h3>
              <p class="text-[10px] font-bold text-toy-muted">自动分配网内 IP 与密钥对</p>
            </div>
          </div>
          <button 
            @click="isAddDeviceModalOpen = false"
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
          >
            ✕
          </button>
        </div>

        <form @submit.prevent="submitAddDevice" class="mt-4 space-y-4">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">设备标识名称</label>
            <input 
              v-model="newDeviceName"
              type="text" 
              placeholder="例如: MacBook-M4-Pro" 
              class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none focus:ring-2 focus:ring-toy-yellow"
              required
              autofocus
            />
          </div>

          <div class="pt-2 flex items-center justify-end gap-2">
            <button 
              type="button" 
              @click="isAddDeviceModalOpen = false"
              class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer"
            >
              取消
            </button>
            <button 
              type="submit" 
              class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer"
            >
              立即生成凭证
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- 弹窗 3: 重命名使用端 -->
    <div v-if="renameTarget" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-5" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <h3 class="text-base font-black text-toy-black">重命名使用端</h3>
        <p class="mt-1 text-[11px] font-bold text-toy-muted">只影响面板显示与导出文件名，WG 公钥/私钥不变。</p>
        <input
          v-model="renameValue"
          type="text"
          class="mt-3 w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none focus:ring-2 focus:ring-toy-yellow"
        />
        <div class="mt-4 flex items-center justify-end gap-2">
          <button @click="renameTarget = null" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitRename" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer">保存</button>
        </div>
      </div>
    </div>

    <!-- 资产中转（doc/12 §8）：面板缓存安装包，供无外网的节点取用 -->
    <div v-if="assetsPanelOpen" class="toy-card p-5 bg-white space-y-4">
      <div class="flex items-center justify-between gap-2 flex-wrap">
        <div>
          <h3 class="text-sm font-black text-toy-black flex items-center gap-2">
            资产中转 <span class="font-mono text-toy-muted">{{ assets.length }}</span>
            <span class="toy-badge bg-toy-cyan-light text-[10px]">离线兜底</span>
          </h3>
          <p class="text-[10px] font-bold text-toy-muted">按测速序下载到面板缓存，再推送到节点 /usr/local/bin/（节点无外网时的安装路径）</p>
        </div>
        <div class="flex items-center gap-2">
          <button @click="loadAssets" class="toy-btn px-2.5 py-1.5 bg-white text-[11px] font-black rounded-xl cursor-pointer">↻ 刷新</button>
          <button @click="openAssetModal()" class="toy-btn px-3 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer">➕ 注册资产</button>
        </div>
      </div>

      <p v-if="assetsLoading" class="text-xs font-bold text-toy-muted">读取资产列表…</p>
      <p v-else-if="!assets.length" class="text-xs font-bold text-toy-muted">
        暂无资产：点「注册资产」填入名称、版本与下载源（支持官方直链/镜像/自有 release）。
      </p>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
              <th class="pb-2">资产</th>
              <th class="pb-2">版本 / 架构</th>
              <th class="pb-2">缓存</th>
              <th class="pb-2">源</th>
              <th class="pb-2 text-right whitespace-nowrap">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-toy-gray">
            <tr v-for="as in assets" :key="as.id" class="align-top">
              <td class="py-3">
                <b class="text-toy-black">{{ as.name }}</b>
                <div v-if="as.note" class="text-[10px] font-bold text-toy-muted">{{ as.note }}</div>
              </td>
              <td class="py-3 font-mono text-[11px] text-toy-black">{{ as.version || '—' }}<template v-if="as.arch"> · {{ as.arch }}</template></td>
              <td class="py-3">
                <span class="toy-badge text-[10px] whitespace-nowrap" :class="as.cached ? 'bg-toy-mint' : 'bg-toy-gray text-toy-muted'">
                  {{ as.cached ? formatBytes(as.size) : '未缓存' }}
                </span>
                <div v-if="as.sha256" class="text-[10px] font-mono text-toy-muted mt-1 break-all max-w-[180px]">{{ as.sha256.slice(0, 24) }}…</div>
              </td>
              <td class="py-3">
                <div v-for="(s, i) in as.sources" :key="i" class="font-mono text-[10px] text-toy-muted break-all max-w-[220px]">{{ s }}</div>
                <div v-if="assetProbe[as.id]?.length" class="mt-1 space-y-0.5">
                  <div v-for="(r, i) in assetProbe[as.id]" :key="i" class="font-mono text-[10px]" :class="r.status ? 'text-emerald-700' : 'text-red-600'">
                    {{ r.status ? `✓ ${r.latency_ms}ms` : '✗ 不可用' }}
                    <span class="text-toy-muted">{{ shortUrl(r.url) }}</span>
                  </div>
                </div>
              </td>
              <td class="py-3">
                <div class="flex items-center justify-end gap-1 flex-wrap">
                  <button @click="probeAsset(as)" :disabled="assetBusyId === as.id" class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-50">测速</button>
                  <button @click="fetchAsset(as)" :disabled="assetBusyId === as.id" class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-50">
                    {{ assetBusyId === as.id ? '…' : '下载缓存' }}
                  </button>
                  <button @click="openPushAsset(as)" :disabled="!as.cached" :title="as.cached ? '' : '先下载缓存后才能推送'" class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-40">推送</button>
                  <button @click="openAssetModal(as)" class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer">编辑</button>
                  <button @click="askDeleteAsset(as)" class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white text-red-600 hover:bg-red-50 cursor-pointer">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 弹窗 4: 组网向导（预检 → 执行） -->
    <div v-if="wizardOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-3xl w-full p-6 space-y-4 max-h-[92vh] overflow-y-auto" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <div>
            <h3 class="text-base font-black text-toy-black">组网向导</h3>
            <p class="text-[10px] font-bold text-toy-muted">先预检（只探测、不落库）再执行；执行会后台建任务并逐台下发配置</p>
          </div>
          <button @click="wizardOpen = false" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>

        <!-- 第 1 步：网络参数 -->
        <div class="space-y-2">
          <h4 class="text-xs font-black text-toy-black">① 网络参数</h4>
          <div class="grid grid-cols-2 lg:grid-cols-4 gap-2">
            <label class="text-[10px] font-bold text-toy-muted">网段
              <input v-model="wizard.network.subnet" placeholder="10.66.66.0/24" :disabled="!!overview.network" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none disabled:opacity-60" />
            </label>
            <label class="text-[10px] font-bold text-toy-muted">中心 IP
              <input v-model="wizard.network.hub_ip" placeholder="10.66.66.1" :disabled="!!overview.network" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none disabled:opacity-60" />
            </label>
            <label class="text-[10px] font-bold text-toy-muted">接口名
              <input v-model="wizard.network.iface" placeholder="wg0" :disabled="!!overview.network" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none disabled:opacity-60" />
            </label>
            <label class="text-[10px] font-bold text-toy-muted">中心端口
              <input v-model.number="wizard.hubPort" type="number" min="1" max="65535" placeholder="51820" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none" />
            </label>
          </div>
          <p v-if="overview.network" class="text-[10px] font-bold text-toy-muted">网络已初始化：网段/中心 IP/接口名只读（改这些要重建网络）。</p>
        </div>

        <!-- 第 2 步：中心与成员 -->
        <div class="space-y-2">
          <h4 class="text-xs font-black text-toy-black">② 中心节点与成员</h4>
          <label class="block text-[10px] font-bold text-toy-muted">中心节点（本机不支持担任中心：面板容器无 NET_ADMIN）
            <select v-model.number="wizard.hubServerId" class="mt-0.5 w-full px-2 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none">
              <option :value="0">— 请选择 —</option>
              <option v-for="s in wizardHubCandidates" :key="s.id" :value="s.id">{{ s.name }}{{ s.ssh_ready ? '' : '（无 SSH 凭据）' }}</option>
            </select>
          </label>
          <div>
            <div class="text-[10px] font-bold text-toy-muted mb-1">成员节点（勾选后由面板下发 spoke 配置）</div>
            <div class="max-h-40 overflow-y-auto p-2 bg-toy-bg border-2 border-toy-black rounded-lg space-y-1">
              <label v-for="s in wizardSpokeCandidates" :key="s.id" class="flex items-center gap-2 text-xs font-bold cursor-pointer">
                <input type="checkbox" :value="s.id" v-model="wizard.spokeIds" class="w-3.5 h-3.5 accent-black" />
                <span class="text-toy-black">{{ s.name }}</span>
                <span v-if="s.is_self" class="toy-badge bg-toy-lavender text-[9px]">本机</span>
                <span v-if="s.wg_role" class="text-[10px] font-mono text-toy-muted">当前角色 {{ s.wg_role }}</span>
                <span v-if="!s.ssh_ready" class="text-[10px] text-toy-orange">无 SSH 凭据</span>
              </label>
              <p v-if="!wizardSpokeCandidates.length" class="text-[11px] font-bold text-toy-muted">没有可用成员节点</p>
            </div>
          </div>
          <p v-if="wizard.network.hub_ip && wizard.network.subnet" class="text-[10px] font-bold text-toy-muted">
            成员 IP 由面板按网段自动分配（中心占 {{ wizard.network.hub_ip }}），预检会同时探测 SSH 与接口冲突。
          </p>
        </div>

        <div class="flex items-center gap-2 flex-wrap">
          <button @click="runPlan" :disabled="wizardPlanning || !wizard.hubServerId" class="toy-btn px-3.5 py-1.5 bg-white text-xs font-black rounded-xl cursor-pointer disabled:opacity-50">
            {{ wizardPlanning ? '预检中（SSH 探测约 40s）…' : '① 预检' }}
          </button>
          <button @click="runApply" :disabled="!plan || plan.blocked || wizardApplying || overview.running_task" class="toy-btn px-3.5 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer disabled:opacity-50">
            {{ wizardApplying ? '提交中…' : '② 执行组网' }}
          </button>
          <span v-if="plan?.blocked" class="text-[11px] font-black text-red-600">预检存在 error 级问题，执行已禁用</span>
          <span v-else-if="plan" class="text-[11px] font-bold text-emerald-700">预检通过，可以执行</span>
        </div>

        <!-- 预检结果 -->
        <div v-if="plan" class="space-y-2">
          <h4 class="text-xs font-black text-toy-black">预检结果</h4>
          <div class="p-2.5 bg-toy-bg border-2 border-toy-black rounded-lg space-y-1">
            <div class="text-[11px] font-black text-toy-black">
              中心：{{ plan.hub.name }}（{{ plan.hub.role === 'hub' ? '现役' : '备援' }} · UDP {{ plan.hub.listen_port }}）
            </div>
            <div v-for="(i, idx) in plan.hub.issues" :key="`h-${idx}`" class="text-[10px] font-bold" :class="i.level === 'error' ? 'text-red-600' : 'text-amber-700'">
              {{ i.level === 'error' ? '✗' : '⚠' }} {{ i.msg }}
            </div>
            <div v-if="!plan.hub.issues.length" class="text-[10px] font-bold text-emerald-700">✓ 中心侧无问题</div>
          </div>
          <div v-for="sp in plan.spokes" :key="sp.server_id" class="p-2.5 bg-toy-bg border-2 border-toy-black rounded-lg space-y-1">
            <div class="text-[11px] font-black text-toy-black">
              成员：{{ sp.name }} · {{ sp.allocated ? sp.wg_ip : '未分配 IP' }}
            </div>
            <div v-for="(i, idx) in sp.issues" :key="`s-${idx}`" class="text-[10px] font-bold" :class="i.level === 'error' ? 'text-red-600' : 'text-amber-700'">
              {{ i.level === 'error' ? '✗' : '⚠' }} {{ i.msg }}
            </div>
            <div v-if="!sp.issues.length" class="text-[10px] font-bold text-emerald-700">✓ 无问题</div>
          </div>
          <p class="text-[10px] font-bold text-toy-muted">
            预检只做探测、不落库不改配置；执行后会建后台任务，逐台下发并可在任务弹窗里看进度。
          </p>
        </div>
      </div>
    </div>

    <!-- 弹窗 5: 导入现有网络 -->
    <div v-if="importOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <div>
            <h3 class="text-base font-black text-toy-black">导入现有网络</h3>
            <p class="text-[10px] font-bold text-toy-muted">读现役中心实况建网、收编已有成员；备援只做 warm standby</p>
          </div>
          <button @click="importOpen = false" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <label class="block text-[10px] font-bold text-toy-muted">现役中心节点
          <select v-model.number="importForm.hub_server_id" class="mt-0.5 w-full px-2 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none">
            <option :value="0">— 请选择 —</option>
            <option v-for="s in wizardHubCandidates" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </label>
        <label class="block text-[10px] font-bold text-toy-muted">备援中心（可选，只读登记）
          <select v-model.number="importForm.standby_server_id" class="mt-0.5 w-full px-2 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none">
            <option :value="0">— 不设备援 —</option>
            <option v-for="s in wizardHubCandidates.filter((x) => x.id !== importForm.hub_server_id)" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </label>
        <div>
          <div class="text-[10px] font-bold text-toy-muted mb-1">候选成员（勾选面板要接管的 spoke 节点）</div>
          <div class="max-h-40 overflow-y-auto p-2 bg-toy-bg border-2 border-toy-black rounded-lg space-y-1">
            <label v-for="s in wizardSpokeCandidates.filter((x) => x.id !== importForm.hub_server_id && x.id !== importForm.standby_server_id)" :key="s.id" class="flex items-center gap-2 text-xs font-bold cursor-pointer">
              <input type="checkbox" :value="s.id" v-model="importForm.candidates" class="w-3.5 h-3.5 accent-black" />
              <span class="text-toy-black">{{ s.name }}</span>
              <span v-if="s.is_self" class="toy-badge bg-toy-lavender text-[9px]">本机</span>
            </label>
          </div>
        </div>
        <p v-if="importErr" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ importErr }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="importOpen = false" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitImport" :disabled="importBusy || !importForm.hub_server_id" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ importBusy ? '提交中…' : '开始导入' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 弹窗 6: 新机接管现役中心 -->
    <div v-if="takeoverOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 space-y-3 max-h-[88vh] overflow-y-auto" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <div>
            <h3 class="text-base font-black text-toy-black">迁移到新机 · 接管现役身份</h3>
            <p class="text-[10px] font-bold text-toy-muted">目标机继承现役的密钥与端口，成员只改端点（比「切换中心」改动更小）</p>
          </div>
          <button @click="takeoverOpen = false" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <p class="text-[11px] font-bold text-toy-muted">
          流程：① 目标机装 WG 并<b class="text-toy-black">继承现役的密钥与端口</b>（身份不变）→ ② 面板侧真握手实测（不通即中止，成员零改动）
          → ③ 金丝雀成员先翻端点验证 → ④ 其余成员翻转（本机最后）→ ⑤ 置现役并停用旧中心。
        </p>
        <p class="text-[11px] font-bold text-toy-muted">
          因为身份不变，<b class="text-toy-black">使用端只差 Endpoint 一行</b>：到中心面板重新下载凭证导入即可
          （对比「切换为现役」＝目标机用自己的密钥，使用端要换一整套 A/B 凭证）。
        </p>
        <label class="block text-[10px] font-bold text-toy-muted">接管目标机（新机器/备援，需已录 SSH）
          <select v-model.number="takeoverForm.target_server_id" class="mt-0.5 w-full px-2 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none">
            <option :value="0">— 请选择 —</option>
            <option v-for="s in takeoverTargets" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </label>
        <div class="grid grid-cols-2 gap-2">
          <label class="text-[10px] font-bold text-toy-muted">监听端口（默认沿用现役；云安全组要放行）
            <input v-model.number="takeoverForm.port" type="number" min="0" max="65535" placeholder="沿用" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none" />
          </label>
          <label class="text-[10px] font-bold text-toy-muted">金丝雀成员（先翻它验证，可选）
            <select v-model.number="takeoverForm.canary_server_id" class="mt-0.5 w-full px-2 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none">
              <option :value="0">— 全部成员 —</option>
              <option v-for="p in canaryCandidates" :key="p.id" :value="p.server_id || 0">{{ p.name }}</option>
            </select>
          </label>
        </div>
        <p class="text-[10px] font-bold text-toy-muted">
          金丝雀必须是 SSH 纳管的服务器成员（接管目标机除外）。目标机若用 WG 网段地址做 SSH 会被跳过（翻转时重启接口会失联）；
          本机成员会自动排在最后执行。目标机若已是本网成员，请先在成员表把它「移出」再接管。
        </p>
        <p v-if="!takeoverTargets.length" class="p-2.5 bg-toy-bg border-2 border-toy-black rounded-xl text-[11px] font-bold text-toy-muted">
          ⚠ 还没有可用的目标机：先去「节点管理」添加一台公网服务器并录入 SSH；
          已在本网内的 ssh 成员要先在成员表移出（备援中心可直接作为目标）。
        </p>
        <p v-if="takeoverErr" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ takeoverErr }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="takeoverOpen = false" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitTakeover" :disabled="takeoverBusy || !takeoverForm.target_server_id" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ takeoverBusy ? '提交中…' : '开始接管' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 弹窗 7: 注册/编辑资产 -->
    <div v-if="assetModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black">{{ assetModal.id ? '编辑资产' : '注册资产' }}</h3>
          <button @click="assetModal = null" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <label class="text-[10px] font-bold text-toy-muted">名称（字母数字与 _ -）
            <input v-model="assetModal.name" placeholder="wireguard-tools" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none" />
          </label>
          <label class="text-[10px] font-bold text-toy-muted">版本
            <input v-model="assetModal.version" placeholder="1.0.20210914" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none" />
          </label>
          <label class="text-[10px] font-bold text-toy-muted">架构
            <input v-model="assetModal.arch" placeholder="amd64" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none" />
          </label>
          <label class="text-[10px] font-bold text-toy-muted">SHA256（可选，校验用）
            <input v-model="assetModal.sha256" placeholder="留空则下载后自动计算" class="mt-0.5 w-full px-2 py-1.5 text-xs font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none" />
          </label>
        </div>
        <label class="block text-[10px] font-bold text-toy-muted">下载源（每行一个 http(s) URL，GitHub 链接会自动生成加速候选）
          <textarea v-model="assetModal.sourcesText" rows="3" placeholder="https://github.com/.../download/v1.0/file" class="mt-0.5 w-full px-2 py-1.5 text-[11px] font-mono font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"></textarea>
        </label>
        <label class="block text-[10px] font-bold text-toy-muted">备注
          <input v-model="assetModal.note" placeholder="这个包给谁用" class="mt-0.5 w-full px-2 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none" />
        </label>
        <p v-if="assetModal.error" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ assetModal.error }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="assetModal = null" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitAsset" :disabled="assetModal.busy" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ assetModal.busy ? '保存中…' : '保存' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 弹窗 8: 推送资产到节点 -->
    <div v-if="pushAsset" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 space-y-3" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <div class="flex items-center justify-between pb-2 border-b-2.5 border-toy-black">
          <h3 class="text-base font-black text-toy-black">推送资产到节点</h3>
          <button @click="pushAsset = null" class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg font-black text-xs cursor-pointer">✕</button>
        </div>
        <p class="text-xs font-bold text-toy-muted">
          把缓存的 <b class="font-mono text-toy-black">{{ pushAsset.name }}-{{ pushAsset.version }}</b> 推到目标节点 /usr/local/bin/。
          目标必须是网内成员。
        </p>
        <select v-model.number="pushServerId" class="w-full px-2 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none">
          <option :value="0">— 请选择成员节点 —</option>
          <option v-for="p in serverPeers" :key="p.id" :value="p.server_id || 0">{{ p.name }}</option>
        </select>
        <p v-if="pushErr" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">{{ pushErr }}</p>
        <div class="pt-1 flex items-center justify-end gap-2">
          <button @click="pushAsset = null" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="submitPushAsset" :disabled="pushBusy || !pushServerId" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60">
            {{ pushBusy ? '推送中…' : '开始推送' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 弹窗 4: 中心节点面板（概览 / 主备就绪度 / 使用端凭证 / SSH 成员） -->
    <div v-if="hubModal" class="fixed inset-0 z-[45] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs">
      <div
        class="bg-white border-3 border-toy-black rounded-2xl max-w-3xl w-full max-h-[88vh] overflow-y-auto"
        :style="{ boxShadow: '6px 6px 0px #121212' }"
        role="dialog"
        aria-modal="true"
        :aria-label="`中心节点面板 ${hubModal.data?.hub.name || hubModal.hubId}`"
      >
        <div class="sticky top-0 z-10 bg-white px-5 pt-5 pb-3 border-b-2.5 border-toy-black flex items-center justify-between gap-2">
          <h3 class="text-base font-black text-toy-black flex items-center gap-2 flex-wrap">
            <span>🛰️</span>
            <span>中心节点 · {{ hubModal.data?.hub.name || `#${hubModal.hubId}` }}</span>
            <span v-if="hubModal.data" class="toy-badge text-[10px]" :class="hubModal.data.hub.is_active ? 'bg-toy-mint' : 'bg-toy-gray'">
              {{ hubModal.data.hub.is_active ? '现役' : '备援' }}
            </span>
          </h3>
          <button
            @click="closeHubModal"
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer shrink-0"
          >✕</button>
        </div>

        <div class="px-5 py-4 space-y-4">
          <p v-if="hubModal.loading" class="text-xs font-bold text-toy-muted">读取中心详情…</p>
          <div v-else-if="!hubModal.data" class="p-3 bg-red-50 border-2 border-red-400 rounded-xl text-xs font-bold text-red-700">
            ⚠ {{ hubModal.error || '中心详情加载失败' }}
          </div>

          <template v-else>
            <!-- 概览 -->
            <div class="grid grid-cols-2 md:grid-cols-3 gap-2">
              <div class="p-2.5 rounded-xl border-2 border-toy-black bg-toy-bg">
                <span class="block text-[10px] font-bold text-toy-muted">监听端点</span>
                <span class="text-xs font-mono font-black text-toy-black break-all">{{ hubModal.data.hub.endpoint || '端点未知' }}</span>
              </div>
              <div class="p-2.5 rounded-xl border-2 border-toy-black bg-toy-bg">
                <span class="block text-[10px] font-bold text-toy-muted">监听端口</span>
                <span class="text-xs font-mono font-black text-toy-black">UDP {{ hubModal.data.hub.listen_port }}</span>
              </div>
              <div class="p-2.5 rounded-xl border-2 border-toy-black bg-toy-bg">
                <span class="block text-[10px] font-bold text-toy-muted">公钥指纹</span>
                <span class="text-xs font-mono font-black text-toy-black break-all" :title="hubModal.data.hub.public_key">{{ hubModal.data.hub.key_fp || '—' }}</span>
              </div>
              <div class="p-2.5 rounded-xl border-2 border-toy-black bg-toy-bg">
                <span class="block text-[10px] font-bold text-toy-muted">本月出向（计费）</span>
                <span class="text-xs font-mono font-black text-toy-black">
                  {{ formatBytes(hubModal.data.hub.month_billed) }}<template v-if="hubModal.data.hub.quota_gb"> / {{ hubModal.data.hub.quota_gb }} GB</template>
                </span>
              </div>
              <div class="p-2.5 rounded-xl border-2 border-toy-black bg-toy-bg">
                <span class="block text-[10px] font-bold text-toy-muted">最近巡检</span>
                <span class="text-xs font-mono font-black text-toy-black">{{ hubModal.data.hub.checked_at ? handshakeAgo(hubModal.data.hub.checked_at) : '未巡检' }}</span>
              </div>
              <div class="p-2.5 rounded-xl border-2 border-toy-black bg-toy-bg">
                <span class="block text-[10px] font-bold text-toy-muted">面板持有私钥</span>
                <span class="text-xs font-mono font-black" :class="hubModal.data.hub.can_render ? 'text-toy-black' : 'text-toy-pink'">
                  {{ hubModal.data.hub.can_render ? '是（可重渲染凭证）' : '否（导入的中心只能读）' }}
                </span>
              </div>
            </div>
            <div class="text-[10px] font-bold text-toy-muted font-mono">
              双向合计 {{ formatBytes(hubModal.data.hub.month_rx + hubModal.data.hub.month_tx) }} · 入向不计费
            </div>
            <div v-if="hubModal.data.hub.quota_gb" class="h-2.5 rounded-full border border-toy-black bg-white overflow-hidden">
              <div
                class="h-full transition-all"
                :class="quotaPct(hubModal.data.hub) >= 100 ? 'bg-toy-pink' : quotaPct(hubModal.data.hub) > 80 ? 'bg-toy-orange' : 'bg-toy-mint'"
                :style="{ width: `${Math.min(100, quotaPct(hubModal.data.hub))}%` }"
              ></div>
            </div>

            <div class="flex items-center gap-2 flex-wrap">
              <button
                @click="probeHubInModal"
                :disabled="hubModal.probing"
                class="toy-btn px-3 py-1.5 bg-white text-[11px] font-black rounded-xl cursor-pointer"
              >{{ hubModal.probing ? '实测中…（面板侧握手）' : '实测 UDP 端口' }}</button>
              <button @click="reloadHub" class="px-3 py-1.5 bg-white border-2 border-toy-black text-[11px] font-black rounded-lg hover:bg-toy-bg cursor-pointer">刷新</button>
              <button
                v-if="hubModal.data.hub.is_active"
                @click="openTakeover"
                :disabled="overview.running_task || !takeoverTargets.length"
                class="px-3 py-1.5 bg-white border-2 border-toy-black text-[11px] font-black rounded-lg hover:bg-toy-bg cursor-pointer disabled:opacity-50"
                title="额度用完换机器：新机继承本中心的密钥与端口，使用端只改 Endpoint 一行"
              >迁移到新机（接管身份）</button>
            </div>
            <div
              v-if="hubModal.probe"
              class="p-3 rounded-xl border-2 text-xs font-bold"
              :class="hubModal.probe.ok ? 'bg-toy-mint/40 border-toy-black text-toy-black' : 'bg-red-50 border-red-400 text-red-700'"
            >
              <template v-if="hubModal.probe.mode === 'handshake'">
                <template v-if="hubModal.probe.ok">
                  ✔ UDP {{ hubModal.data.hub.listen_port }} 实测可达：面板侧握手成功（{{ hubModal.probe.rtt_ms }}ms，探针成员 {{ hubModal.probe.peer }}）
                </template>
                <template v-else>⚠ {{ hubModal.probe.hint || '握手无回应' }}</template>
              </template>
              <template v-else>
                ⚠ {{ hubModal.probe.hint || `粗判结果：${hubModal.probe.reach}` }}
                <span class="font-mono text-[10px] text-toy-muted">（无面板托管成员私钥，只能粗判：{{ hubModal.probe.reason || '—' }}）</span>
              </template>
            </div>
            <p v-if="!hubModal.data.hub.is_active" class="text-[11px] font-bold text-toy-muted">
              想让这台接管现役？用卡片上的「切换为现役」（两阶段金丝雀）。
            </p>

            <!-- 主备就绪度 -->
            <div v-if="hubModal.data.hubs_readiness.length > 1" class="space-y-2">
              <h4 class="text-xs font-black text-toy-black">主备中心就绪度</h4>
              <div
                v-for="r in hubModal.data.hubs_readiness"
                :key="`ready-${r.server_id}`"
                class="flex items-center gap-2 flex-wrap p-2.5 rounded-xl border border-toy-black bg-toy-bg"
              >
                <span class="toy-badge text-[10px]" :class="r.is_active ? 'bg-toy-mint' : 'bg-gray-100'">{{ r.is_active ? 'A · 现役' : 'B · 备援' }}</span>
                <b class="text-xs font-black text-toy-black">{{ r.name }}</b>
                <span class="text-[11px] font-mono text-toy-muted">{{ r.endpoint || '端点未知' }}</span>
                <span class="toy-badge text-[10px]" :class="r.ready ? 'bg-toy-mint' : 'bg-toy-orange text-white'">{{ r.ready ? '就绪' : r.missing }}</span>
                <button
                  @click="provisionHubSlot(r)"
                  :disabled="overview.running_task || !hubSlotServer(r).ssh_ready"
                  :title="hubSlotServer(r).ssh_ready ? '' : '该节点没有可用的 SSH 凭据'"
                  class="ml-auto px-2.5 py-1 bg-white border border-toy-black rounded-lg text-[10px] font-black hover:bg-toy-yellow cursor-pointer disabled:opacity-50"
                >{{ r.ready ? '重新下发' : '一键配置' }}</button>
              </div>
              <p class="text-[10px] font-bold text-toy-muted">
                两台都就绪时，使用端凭证有 A/B 两份：切换中心后导入另一份即可（密钥不变，只有端点一行不同）。
                「一键配置」由面板生成/重写配置并经 SSH 下发；唯一要人工的是到云控制台安全组放行该中心监听端口的 UDP。
              </p>
            </div>

            <!-- 使用端凭证 -->
            <div class="space-y-2">
              <div class="flex items-center justify-between flex-wrap gap-2">
                <h4 class="text-xs font-black text-toy-black">使用端凭证（{{ hubModal.data.creds.length }}）</h4>
                <div class="flex items-center gap-2">
                  <button
                    @click="openAddDeviceModal"
                    :disabled="overview.running_task"
                    class="px-2.5 py-1 bg-white border border-toy-black rounded-lg text-[10px] font-black hover:bg-toy-yellow cursor-pointer disabled:opacity-50"
                  >➕ 新增使用端</button>
                  <button
                    @click="downloadCreds"
                    class="px-2.5 py-1 bg-white border border-toy-black rounded-lg text-[10px] font-black hover:bg-toy-yellow cursor-pointer"
                  >下载全部（zip）</button>
                </div>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full text-left text-xs">
                  <thead>
                    <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[10px]">
                      <th class="pb-1.5">名称</th>
                      <th class="pb-1.5">WG IP</th>
                      <th class="pb-1.5">状态</th>
                      <th class="pb-1.5">最近握手</th>
                      <th class="pb-1.5 text-right">操作</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-toy-gray">
                    <tr v-for="p in hubModal.data.creds" :key="`cred-${p.id}`">
                      <td class="py-2 font-bold text-toy-black">
                        {{ p.name }}
                        <span v-if="!p.can_export" class="toy-badge bg-gray-100 text-[9px]" title="导入成员：面板没有私钥，无法导出凭证">导入</span>
                      </td>
                      <td class="py-2 font-mono">{{ p.wg_ip }}</td>
                      <td class="py-2"><span class="toy-badge text-[10px]" :class="peerStatusTag(p).cls">{{ peerStatusTag(p).text }}</span></td>
                      <td class="py-2 font-mono text-toy-muted">{{ p.last_handshake ? handshakeAgo(p.last_handshake) : '从未' }}</td>
                      <td class="py-2 text-right space-x-1 font-sans whitespace-nowrap">
                        <template v-if="hubModal.renamingId === p.id">
                          <input
                            v-model="hubModal.renameText"
                            class="px-2 py-0.5 text-[11px] font-bold border-2 border-toy-black rounded-lg bg-toy-bg w-32 focus:outline-none"
                            @keydown.enter="submitHubRename(p)"
                          />
                          <button @click="submitHubRename(p)" class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-toy-yellow cursor-pointer">保存</button>
                          <button @click="hubModal.renamingId = 0" class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white cursor-pointer">取消</button>
                        </template>
                        <template v-else>
                          <button
                            v-if="p.can_export"
                            @click="viewDeviceConfig(p, hubModal.hubId)"
                            class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-toy-yellow hover:bg-yellow-300 cursor-pointer"
                          >凭证 / QR</button>
                          <button
                            v-else
                            @click="reissuePeer(p)"
                            class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer"
                            title="导入成员：面板没有它的私钥，出不了二维码。点这里换一套面板托管的密钥（同 IP 同 PSK）并在所有中心替换"
                          >生成二维码</button>
                          <button
                            @click="startHubRename(p)"
                            class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer"
                          >重命名</button>
                          <button
                            @click="syncPeerHubs(p)"
                            :disabled="hubModal.syncingId === p.id || overview.running_task"
                            class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-cyan cursor-pointer disabled:opacity-50"
                          >{{ hubModal.syncingId === p.id ? '下发中…' : '同步到所有中心' }}</button>
                          <button
                            @click="handleDeletePeer(p)"
                            :disabled="overview.running_task"
                            class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-red-50 text-red-600 cursor-pointer disabled:opacity-50"
                          >移出</button>
                        </template>
                      </td>
                    </tr>
                    <tr v-if="!hubModal.data.creds.length">
                      <td colspan="5" class="py-5 text-center text-toy-muted font-bold">暂无使用端。点「新增使用端」生成第一份凭证（手机/笔记本扫码即用）。</td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <p v-if="hubModal.data.creds.some((p) => !p.can_export)" class="text-[10px] font-bold text-toy-muted">
                标「导入」的使用端是面板从现网上收编的：只有公钥，没有私钥，出不了二维码。
                点「生成二维码」会用同 IP、同 PSK 换一套面板托管的密钥并在所有中心替换，设备按新码重新导入即可（期间该设备会短暂失联）。
              </p>
            </div>

            <!-- 成员（SSH 服务器节点） -->
            <div v-if="hubModal.data.members.length" class="space-y-2">
              <h4 class="text-xs font-black text-toy-black">成员 · SSH 节点（{{ hubModal.data.members.length }}）</h4>
              <div class="overflow-x-auto">
                <table class="w-full text-left text-xs">
                  <thead>
                    <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[10px]">
                      <th class="pb-1.5">名称</th>
                      <th class="pb-1.5">WG IP</th>
                      <th class="pb-1.5">状态</th>
                      <th class="pb-1.5">收 / 发</th>
                      <th class="pb-1.5 text-right">操作</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-toy-gray">
                    <tr v-for="p in hubModal.data.members" :key="`mem-${p.id}`">
                      <td class="py-2 font-bold text-toy-black">{{ p.name }}</td>
                      <td class="py-2 font-mono">{{ p.wg_ip }}</td>
                      <td class="py-2"><span class="toy-badge text-[10px]" :class="peerStatusTag(p).cls">{{ peerStatusTag(p).text }}</span></td>
                      <td class="py-2 font-mono text-toy-muted">{{ formatBytes(p.rx_bytes) }} / {{ formatBytes(p.tx_bytes) }}</td>
                      <td class="py-2 text-right space-x-1 font-sans whitespace-nowrap">
                        <button
                          @click="verifyPeer(p)"
                          :disabled="verifyingId === p.id || overview.running_task"
                          class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-cyan cursor-pointer disabled:opacity-50"
                        >{{ verifyingId === p.id ? '验证中…' : '验证' }}</button>
                        <button
                          v-if="p.can_export"
                          @click="downloadPeerConf(p)"
                          class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-bg cursor-pointer"
                        >下载 .conf</button>
                        <button
                          @click="handleDeletePeer(p)"
                          :disabled="overview.running_task"
                          class="px-2 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-red-50 text-red-600 cursor-pointer disabled:opacity-50"
                        >移出</button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <div v-if="hubModal.error" class="p-3 bg-red-50 border-2 border-red-400 rounded-xl text-xs font-bold text-red-700">⚠ {{ hubModal.error }}</div>
          </template>
        </div>
      </div>
    </div>

    <!-- 风格内确认弹窗（替代原生 confirm） -->
    <div v-if="confirmText" class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-5" :style="{ boxShadow: '6px 6px 0px #121212' }" role="alertdialog" aria-modal="true">
        <h3 class="text-base font-black text-toy-black flex items-center gap-2"><span>⚠️</span>请确认</h3>
        <p class="mt-2 text-xs font-bold text-toy-muted">{{ confirmText }}</p>
        <div class="mt-4 flex items-center justify-end gap-2">
          <button @click="confirmText = ''" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="confirmYes" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-pink text-white text-xs font-black cursor-pointer">确认</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue';
import QRCode from 'qrcode';
import type {
  WGOverviewData,
  WGTask,
  WGPlanInput,
  WGPlanResp,
  WGHubDetailData,
  WGHubProbeResp,
  WGSyncHubsResp,
  WGAssetItem,
  WGAssetProbeResult,
} from '../../../types/backend';
import { formatBytes } from '../../../services/format';
import { api } from '../../../services/api';

type WGPeer = WGOverviewData["peers"][number];
type WGHub = WGOverviewData["hubs"][number];
type WGServer = WGOverviewData["servers"][number];

const emptyOverview = (): WGOverviewData => ({
  network: null,
  hubs: [],
  retired_hubs: [],
  peers: [],
  servers: [],
  running_task: false,
});

const overview = ref<WGOverviewData>(emptyOverview());

const isLoading = ref(true);
const isPatrolling = ref(false);
const hubLatency = ref<Record<number, number>>({});
const errorMsg = ref('');
const okMsg = ref('');
const verifyingId = ref<number | null>(null);
const busyServerId = ref<number | null>(null);

// ---------- 任务进度弹窗（切换中心 / 配置备援 / 接管都落 wg_task） ----------
const taskModal = ref<WGTask | null>(null);
const taskModalDone = ref(false);
let taskTimer: ReturnType<typeof setTimeout> | null = null;

const taskModalStatusText = computed(() => {
  if (!taskModal.value) return '';
  const t = taskModal.value;
  if (t.status === 'success') return '任务已完成，页面数据已刷新。';
  if (t.status === 'failed') return `任务失败：${t.result || '详见步骤日志'}`;
  return '正在执行…（每 2 秒刷新一次进度）';
});

const serverPeers = computed(() => overview.value.peers.filter((p) => p.kind === 'server'));
const devicePeers = computed(() => overview.value.peers.filter((p) => p.kind !== 'server'));

const activeHub = computed(() => overview.value.hubs.find((h) => h.is_active));
const standbyHubs = computed(() => overview.value.hubs.filter((h) => !h.is_active));
// 中心区排序：现役永远第一张，备援在后——三张卡走同一份模板，不再为第三台准备降级版
const orderedHubs = computed(() => [...overview.value.hubs].sort((a, b) => Number(b.is_active) - Number(a.is_active)));

// 网外未纳管的服务器（排除已退役中心：它们单独一段展示）
const unmanagedServers = computed<WGServer[]>(() => {
  const retired = new Set(overview.value.retired_hubs.map((r) => r.server_id));
  return overview.value.servers.filter((s) => !s.in_network && !retired.has(s.id));
});

const selfServer = computed<WGServer | undefined>(() => overview.value.servers.find((s) => s.is_self));

// ---------- 拓扑：按真实成员渲染（中心固定，成员沿椭圆分布） ----------
const topoNodes = computed(() => {
  const list = overview.value.peers;
  const total = list.length;
  if (!total) return [] as { key: string; name: string; wg_ip: string; kind: string; role: string; x: number; y: number }[];
  const rx = 245;
  const ry = 84;
  const cx = 360;
  const cy = 135;
  return list.map((p, i) => {
    const angle = Math.PI * (i / Math.max(total - 1, 1)) - Math.PI / 2; // -90° → +90°
    const side = i % 2 === 0 ? -1 : 1;
    const x = cx + side * Math.abs(Math.cos(angle)) * rx * 0.9 + (side > 0 ? -25 : 25);
    const y = cy + Math.sin(angle) * ry + (i % 2 === 0 ? -8 : 8);
    const role = activeHub.value && p.server_id === activeHub.value.server_id
      ? 'hub'
      : standbyHubs.value.some((h) => h.server_id === p.server_id) ? 'standby' : 'spoke';
    return {
      key: `p-${p.id}`,
      name: p.name,
      wg_ip: p.wg_ip,
      kind: p.kind,
      role,
      x: Math.max(70, Math.min(650, x)),
      y: Math.max(36, Math.min(234, y)),
    };
  });
});

function shortName(name: string): string {
  return name.length > 11 ? name.slice(0, 10) + '…' : name;
}

function handshakeAgo(ts: number | null): string {
  if (!ts) return '从未';
  const diff = Math.floor(Date.now() / 1000) - ts;
  if (diff < 0) return '刚刚';
  if (diff < 60) return `${diff} 秒前`;
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  return `${Math.floor(diff / 86400)} 天前`;
}

// 成员状态：握手正常 / 巡检失败原文 / 待巡检
function peerStatusTag(p: WGPeer): { text: string; cls: string } {
  if (p.status === 'online') return { text: '● 在线', cls: 'bg-toy-mint text-toy-black' };
  if (p.status === 'left') return { text: '○ 已移出', cls: 'bg-gray-200 text-toy-muted' };
  const text = p.last_error
    ? (p.last_error.length > 22 ? p.last_error.slice(0, 21) + '…' : p.last_error)
    : '待巡检';
  return { text: `○ ${text}`, cls: 'bg-gray-200 text-toy-muted' };
}

// hub 槽位状态（ready/ok=就绪，error=异常，pending=下发中）
function hubStatusTag(h: WGHub): { text: string; cls: string } {
  const s = (h.status || '').toLowerCase();
  if (s === 'ready' || s === 'ok' || s === 'active' || s === 'standby') {
    return { text: '● 就绪', cls: 'bg-toy-mint text-toy-black' };
  }
  if (s === 'error' || s === 'failed') return { text: '⚠ 异常', cls: 'bg-toy-pink text-white' };
  if (s === 'pending') return { text: '⏳ 下发中', cls: 'bg-toy-yellow text-toy-black' };
  return { text: s || '未知', cls: 'bg-toy-gray text-toy-muted' };
}

function stepStatusTag(s: string): { text: string; cls: string } {
  if (s === 'success' || s === 'ok') return { text: '✔ 完成', cls: 'bg-toy-mint text-toy-black' };
  if (s === 'failed' || s === 'error') return { text: '✕ 失败', cls: 'bg-toy-pink text-white' };
  if (s === 'running') return { text: '⏳ 执行中', cls: 'bg-toy-yellow text-toy-black' };
  return { text: '待执行', cls: 'bg-toy-gray text-toy-muted' };
}

// 额度占用百分比（后端 quota_gb 是 GB，流量按字节；口径同 classic：出向=billed）
function quotaPct(h: WGHub): number {
  if (!h.quota_gb) return 0;
  return (h.month_billed / (h.quota_gb * 1e9)) * 100;
}

async function loadData() {
  try {
    overview.value = await api.getWGOverview();
    errorMsg.value = '';
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '读取组网信息失败';
  } finally {
    isLoading.value = false;
  }
}

// ---------- 后台任务进度（2s 轮询，完成后刷新总览） ----------
async function pollTask(taskId: number) {
  try {
    const t = await api.getWGTask(taskId);
    taskModal.value = t;
    const done = t.status === 'success' || t.status === 'failed';
    taskModalDone.value = done;
    if (done) {
      if (t.status === 'failed') errorMsg.value = `任务失败：${t.result || t.status}`;
      await loadData();
      return;
    }
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '读取任务进度失败';
  }
  taskTimer = setTimeout(() => void pollTask(taskId), 2000);
}

function startTaskTracking(taskId: number) {
  taskModal.value = {
    id: taskId, kind: '', status: 'running', result: '', created_at: 0, finished_at: null, steps: [],
  };
  taskModalDone.value = false;
  void pollTask(taskId);
}

function closeTaskModal() {
  if (taskTimer) clearTimeout(taskTimer);
  taskTimer = null;
  taskModal.value = null;
}

async function handleSwitchHub(hubServerId: number) {
  errorMsg.value = '';
  const hub = overview.value.hubs.find((h) => h.server_id === hubServerId);
  confirmText.value = `确认将「${hub?.name ?? hubServerId}」切换为现役主中心？系统将金丝雀两阶段零停机切换，进度在后台任务中执行。`;
  confirmAction = async () => {
    try {
      const r = await api.switchWGHub(hubServerId);
      if (r.task_id) {
        closeHubModal();
        startTaskTracking(r.task_id);
      } else await loadData();
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '切换发起失败';
    }
  };
}

async function probeHub(hubServerId: number) {
  try {
    const res = await api.probeWGHub(hubServerId);
    if (res.ok && res.rtt_ms !== undefined) {
      hubLatency.value[hubServerId] = res.rtt_ms;
    } else {
      errorMsg.value = res.hint || res.reason || '探测未通过';
    }
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '探测失败';
  }
}

// ---------- 中心节点面板（点卡片打开：概览 / 主备就绪度 / 使用端凭证 / SSH 成员） ----------
const hubModal = ref<{
  hubId: number;
  data: WGHubDetailData | null;
  loading: boolean;
  error: string;
  probing: boolean;
  probe: WGHubProbeResp | null;
  syncingId: number;
  renamingId: number;
  renameText: string;
} | null>(null);

async function fetchHubDetail(hubId: number) {
  try {
    const d = await api.getWGHubDetail(hubId);
    if (!hubModal.value || hubModal.value.hubId !== hubId) return; // 已关闭或已切到别的中心：丢弃旧响应
    hubModal.value.data = d;
    hubModal.value.error = '';
  } catch (e) {
    if (!hubModal.value || hubModal.value.hubId !== hubId) return;
    hubModal.value.error = e instanceof Error ? e.message : '中心详情加载失败';
  } finally {
    if (hubModal.value && hubModal.value.hubId === hubId) hubModal.value.loading = false;
  }
}

function openHub(h: WGHub) {
  hubModal.value = {
    hubId: h.server_id, data: null, loading: true, error: '',
    probing: false, probe: null, syncingId: 0, renamingId: 0, renameText: '',
  };
  void fetchHubDetail(h.server_id);
}

function closeHubModal() {
  hubModal.value = null;
}

async function reloadHub() {
  const m = hubModal.value;
  if (!m) return;
  m.loading = true;
  await fetchHubDetail(m.hubId);
}

// 面板里的写操作会同时改总览与面板快照，两边一起刷
async function refreshHubIfOpen() {
  if (!hubModal.value) return;
  await fetchHubDetail(hubModal.value.hubId);
}

async function probeHubInModal() {
  const m = hubModal.value;
  if (!m) return;
  m.probing = true;
  m.probe = null;
  try {
    m.probe = await api.probeWGHub(m.hubId);
  } catch (e) {
    m.error = e instanceof Error ? e.message : '实测失败';
  } finally {
    if (hubModal.value) hubModal.value.probing = false;
  }
}

async function syncPeerHubs(p: WGPeer) {
  const m = hubModal.value;
  if (!m) return;
  m.syncingId = p.id;
  m.error = '';
  try {
    const r: WGSyncHubsResp = await api.syncWGPeerHubs(p.id);
    const bad = (r.results || []).filter((x) => !x.ok);
    if (bad.length) {
      errorMsg.value = `「${p.name}」${r.synced}/${r.total} 台中心下发成功，失败：${bad.map((b) => b.name || b.server_id).join('、')}`;
    } else {
      okMsg.value = `「${p.name}」已下发到全部 ${r.total} 台中心`;
    }
    await Promise.all([reloadHub(), loadData()]);
  } catch (e) {
    m.error = e instanceof Error ? e.message : '同步失败';
  } finally {
    if (hubModal.value) hubModal.value.syncingId = 0;
  }
}

function startHubRename(p: WGPeer) {
  if (!hubModal.value) return;
  hubModal.value.renamingId = p.id;
  hubModal.value.renameText = p.name;
}

async function submitHubRename(p: WGPeer) {
  const m = hubModal.value;
  if (!m || !m.renameText.trim()) return;
  try {
    await api.renameWGPeer(p.id, m.renameText.trim());
    m.renamingId = 0;
    okMsg.value = '已重命名（WG 密钥不变，导出文件名随之更新）';
    await Promise.all([reloadHub(), loadData()]);
  } catch (e) {
    m.error = e instanceof Error ? e.message : '重命名失败';
  }
}

async function downloadPeerConf(p: WGPeer) {
  try {
    const r = await api.getWGDeviceConf(p.id, hubModal.value?.hubId ?? 0);
    const blob = new Blob([r.conf], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = r.filename || 'wg0.conf';
    a.click();
    URL.revokeObjectURL(url);
  } catch (e) {
    if (hubModal.value) hubModal.value.error = e instanceof Error ? e.message : '导出失败';
  }
}

// 就绪度行只有 server_id/name，SSH 就绪与否要去总览的服务器清单里查
function hubSlotServer(r: { server_id: number; name: string }): WGServer {
  return overview.value.servers.find((s) => s.id === r.server_id)
    ?? { id: r.server_id, name: r.name, is_self: false, in_network: true, ssh_ready: false, wg_role: 'standby' };
}

function provisionHubSlot(r: { server_id: number; name: string }) {
  provisionServer(hubSlotServer(r));
}

async function handlePatrol() {
  isPatrolling.value = true;
  try {
    await api.patrol();
    await loadData();
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '巡检失败';
  } finally {
    isPatrolling.value = false;
  }
}

// ---------- 网外节点：配置为备援 / 只读登记 / 纳管本机 ----------
async function provisionServer(s: WGServer) {
  confirmText.value = `确认把「${s.name}」配置为备援中心？面板会生成密钥、下发全部成员并拉起接口，整链路在后台任务里执行。`;
  confirmAction = async () => {
    busyServerId.value = s.id;
    try {
      const r = await api.provisionWGHub(s.id, 0);
      if (r.task_id) {
        closeHubModal();
        startTaskTracking(r.task_id);
      }
      await loadData();
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '配置备援失败';
    } finally {
      busyServerId.value = null;
    }
  };
}

async function registerStandby(s: WGServer) {
  confirmText.value = `确认只读登记「${s.name}」为备援中心？面板只读取该机已有 WG 配置（公钥/端口/端点）登记入库，不写入目标机任何文件。`;
  confirmAction = async () => {
    busyServerId.value = s.id;
    try {
      const r = await api.registerWGStandby(s.id);
      const udp = r.udp;
      okMsg.value = `已登记备援：${r.endpoint}（UDP ${r.listen_port}）`
        + (udp?.checked ? (udp.ok ? ' · UDP 实测可达' : ` · UDP 实测异常：${udp.hint || '未知原因'}`) : '');
      await loadData();
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '登记备援失败';
    } finally {
      busyServerId.value = null;
    }
  };
}

async function adoptServer(s: WGServer) {
  confirmText.value = `确认纳管本机「${s.name}」入网？面板将经宿主 SSH 拉起成员接口并把本机接入专用网段。`;
  confirmAction = async () => {
    busyServerId.value = s.id;
    try {
      const r = await api.adoptWGServer(s.id);
      okMsg.value = `本机已纳管：${r.detail || '接口已拉起'}`;
      await loadData();
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '纳管失败';
    } finally {
      busyServerId.value = null;
    }
  };
}

async function cleanupRetired(r: { server_id: number; name: string }) {
  confirmText.value = `确认清理已退役中心「${r.name}」的记录？机器上的配置在退役时已按时间戳备份（.removed.*），此操作只清库表登记。`;
  confirmAction = async () => {
    try {
      await api.cleanupWGHub(r.server_id);
      okMsg.value = `已清理退役中心记录：${r.name}`;
      await loadData();
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '清理失败';
    }
  };
}

// ---------- 成员操作：验证 / 重命名 / 重新签发 / 移出 ----------
async function verifyPeer(p: WGPeer) {
  verifyingId.value = p.id;
  try {
    const r = await api.verifyWGPeer(p.id);
    if (r.online) okMsg.value = `「${p.name}」复验通过：${r.detail || '接口与握手正常'}`;
    else errorMsg.value = `「${p.name}」复验未通过：${r.detail || '未探测到握手'}`;
    await Promise.all([loadData(), refreshHubIfOpen()]);
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '复验失败';
  } finally {
    verifyingId.value = null;
  }
}

const renameTarget = ref<WGPeer | null>(null);
const renameValue = ref('');

function renamePeer(p: WGPeer) {
  renameTarget.value = p;
  renameValue.value = p.name;
}

async function submitRename() {
  const t = renameTarget.value;
  if (!t || !renameValue.value.trim()) return;
  try {
    await api.renameWGPeer(t.id, renameValue.value.trim());
    renameTarget.value = null;
    okMsg.value = '已重命名（WG 密钥不变，导出文件名随之更新）';
    await Promise.all([loadData(), refreshHubIfOpen()]);
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '重命名失败';
  }
}

async function reissuePeer(p: WGPeer) {
  confirmText.value = `确认重新签发「${p.name}」的凭证？面板会生成托管密钥并在所有未退役中心整体替换（同 IP、同 PSK，改的是公钥），随后可用二维码重新导入。`;
  confirmAction = async () => {
    try {
      const r = await api.reissueWGPeer(p.id);
      okMsg.value = `已重新签发「${p.name}」${r.warn ? ` · ${r.warn}` : ''}`;
      await Promise.all([loadData(), refreshHubIfOpen()]);
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '重新签发失败';
    }
  };
}

// 客户端设备与二维码弹窗
const selectedDevice = ref<WGPeer | null>(null);
const currentConfText = ref('');
const currentConfFile = ref('wg0.conf');
const qrCanvasRef = ref<HTMLCanvasElement | null>(null);
const isCopied = ref(false);

async function viewDeviceConfig(dev: WGPeer, hubId = 0) {
  selectedDevice.value = dev;
  isCopied.value = false;
  try {
    const r = await api.getWGDeviceConf(dev.id, hubId);
    currentConfText.value = r.conf;
    currentConfFile.value = r.filename || `${dev.name}-wg0.conf`;
  } catch (e) {
    currentConfText.value = '';
    errorMsg.value = e instanceof Error ? e.message : '读取配置失败（导入成员不持私钥，无法导出）';
    return;
  }

  await nextTick();
  if (qrCanvasRef.value && currentConfText.value) {
    QRCode.toCanvas(qrCanvasRef.value, currentConfText.value, {
      width: 160,
      margin: 1,
      color: { dark: '#121212', light: '#FFFFFF' }
    });
  }
}

function copyConfText() {
  navigator.clipboard.writeText(currentConfText.value);
  isCopied.value = true;
  setTimeout(() => { isCopied.value = false; }, 2000);
}

function downloadConfFile() {
  if (!selectedDevice.value) return;
  const blob = new Blob([currentConfText.value], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = currentConfFile.value;
  a.click();
  URL.revokeObjectURL(url);
}

// 风格内确认弹窗（替代原生 confirm）
const confirmText = ref('');
let confirmAction: (() => void | Promise<void>) | null = null;
function confirmYes() {
  confirmText.value = '';
  const a = confirmAction;
  confirmAction = null;
  if (a) void a();
}

async function handleDeletePeer(p: WGPeer) {
  confirmText.value = p.kind === 'server'
    ? `确认把服务器成员「${p.name}」移出网络？面板会撤销该节点的 peer 并停止其 WG 隧道。`
    : `确认移除外部设备「${p.name}」的凭据？移除后该设备将无法通过此密钥接入专用网段。`;
  confirmAction = async () => {
    try {
      await api.deleteWGPeer(p.id);
      if (selectedDevice.value?.id === p.id) selectedDevice.value = null;
      await Promise.all([loadData(), refreshHubIfOpen()]);
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '移除失败';
    }
  };
}

// 新增设备弹窗
const isAddDeviceModalOpen = ref(false);
const newDeviceName = ref('');

function openAddDeviceModal() {
  newDeviceName.value = '';
  isAddDeviceModalOpen.value = true;
}

async function submitAddDevice() {
  if (!newDeviceName.value.trim()) return;
  try {
    const dev = await api.addWGDevice(newDeviceName.value.trim());
    isAddDeviceModalOpen.value = false;
    if (dev.warn) errorMsg.value = dev.warn;
    await Promise.all([loadData(), refreshHubIfOpen()]);
    const created = devicePeers.value.find((d) => d.id === dev.id);
    if (created) await viewDeviceConfig(created);
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '生成设备凭据失败';
  }
}

// 任务进行中时页面低频自动刷新（后台任务改的是中心/成员，别的标签页也在看）
let idleTimer: ReturnType<typeof setInterval> | null = null;

// ================= 组网向导（预检 → 执行） =================
type WizardForm = {
  network: { subnet: string; hub_ip: string; iface: string; keepalive: number; mtu: number };
  hubServerId: number;
  hubPort: number;
  spokeIds: number[];
};

const wizardOpen = ref(false);
const wizardPlanning = ref(false);
const wizardApplying = ref(false);
const plan = ref<WGPlanResp | null>(null);
const wizardErr = ref('');
const wizard = ref<WizardForm>({
  network: { subnet: '10.66.66.0/24', hub_ip: '10.66.66.1', iface: 'wg0', keepalive: 25, mtu: 1420 },
  hubServerId: 0,
  hubPort: 51820,
  spokeIds: [],
});

// 中心候选：本机不能当中心（面板容器无 NET_ADMIN），后端也会拒
const wizardHubCandidates = computed(() => overview.value.servers.filter((s) => !s.is_self));
const wizardSpokeCandidates = computed(() =>
  overview.value.servers.filter((s) => s.id !== wizard.value.hubServerId && s.id !== importForm.value.hub_server_id && s.id !== importForm.value.standby_server_id),
);

// 接管目标：有可用 SSH 凭据且不是现役；已在网内的 spoke 成员要先移出，备援本身即现成目标
const takeoverTargets = computed(() => overview.value.servers.filter(
  (x) => x.ssh_ready && x.id !== activeHub.value?.server_id && (x.wg_role === 'standby' || !x.in_network),
));
// 金丝雀候选：spoke 服务器成员里排除本机（本机必须最后翻）
const canaryCandidates = computed(() => {
  const allowed = new Set(overview.value.servers.filter((x) => x.wg_role === 'spoke' && !x.is_self).map((x) => x.id));
  return serverPeers.value.filter((p) => p.server_id && allowed.has(p.server_id));
});

function openWizard() {
  const net = overview.value.network;
  wizard.value = {
    network: net
      ? { subnet: net.subnet, hub_ip: net.hub_ip, iface: net.iface, keepalive: net.keepalive, mtu: net.mtu }
      : { subnet: '10.66.66.0/24', hub_ip: '10.66.66.1', iface: 'wg0', keepalive: 25, mtu: 1420 },
    hubServerId: net?.active_hub_server_id || 0,
    hubPort: overview.value.hubs.find((h) => h.is_active)?.listen_port || 51820,
    spokeIds: overview.value.peers.filter((p) => p.kind === 'server' && p.server_id).map((p) => p.server_id as number),
  };
  plan.value = null;
  wizardErr.value = '';
  wizardOpen.value = true;
}

function wizardPayload(): WGPlanInput {
  const net = overview.value.network;
  return {
    // 网络已存在时不带 network：后端以库里的网络为准（网段/接口名不支持热改）
    network: net
      ? undefined
      : {
          subnet: wizard.value.network.subnet.trim(),
          hub_ip: wizard.value.network.hub_ip.trim(),
          iface: wizard.value.network.iface.trim(),
          keepalive: wizard.value.network.keepalive,
          mtu: wizard.value.network.mtu,
        },
    hub_server_id: wizard.value.hubServerId,
    hub_port: wizard.value.hubPort,
    spokes: wizard.value.spokeIds.map((id) => ({ server_id: id })),
  };
}

async function runPlan() {
  wizardPlanning.value = true;
  wizardErr.value = '';
  plan.value = null;
  try {
    plan.value = await api.wgPlan(wizardPayload());
  } catch (e) {
    wizardErr.value = e instanceof Error ? e.message : '预检失败';
  } finally {
    wizardPlanning.value = false;
  }
}

async function runApply() {
  if (!plan.value || plan.value.blocked) return;
  wizardApplying.value = true;
  wizardErr.value = '';
  try {
    const r = await api.wgApply(wizardPayload());
    wizardOpen.value = false;
    startTaskTracking(r.task_id);
  } catch (e) {
    wizardErr.value = e instanceof Error ? e.message : '执行失败';
  } finally {
    wizardApplying.value = false;
  }
}

// ================= 导入现有网络 =================
const importOpen = ref(false);
const importBusy = ref(false);
const importErr = ref('');
const importForm = ref<{ hub_server_id: number; standby_server_id: number; candidates: number[] }>({
  hub_server_id: 0,
  standby_server_id: 0,
  candidates: [],
});

function openImport() {
  importForm.value = { hub_server_id: 0, standby_server_id: 0, candidates: [] };
  importErr.value = '';
  importOpen.value = true;
}

async function submitImport() {
  if (!importForm.value.hub_server_id) return;
  importBusy.value = true;
  importErr.value = '';
  try {
    const r = await api.wgImport({
      hub_server_id: importForm.value.hub_server_id,
      standby_server_id: importForm.value.standby_server_id || 0,
      candidates: importForm.value.candidates,
    });
    importOpen.value = false;
    startTaskTracking(r.task_id);
  } catch (e) {
    importErr.value = e instanceof Error ? e.message : '导入失败';
  } finally {
    importBusy.value = false;
  }
}

// ================= 新机接管现役中心 =================
const takeoverOpen = ref(false);
const takeoverBusy = ref(false);
const takeoverErr = ref('');
const takeoverForm = ref<{ target_server_id: number; port: number; canary_server_id: number }>({
  target_server_id: 0,
  port: 0,
  canary_server_id: 0,
});

function openTakeover() {
  // 端口默认沿用现役（云安全组照旧放行同一个口，成员改动面最小）
  const first = takeoverTargets.value[0];
  takeoverForm.value = {
    target_server_id: first ? first.id : 0,
    port: activeHub.value?.listen_port || 51820,
    canary_server_id: 0,
  };
  takeoverErr.value = '';
  takeoverOpen.value = true;
}

async function submitTakeover() {
  if (!takeoverForm.value.target_server_id) return;
  takeoverBusy.value = true;
  takeoverErr.value = '';
  try {
    const r = await api.wgTakeover({
      target_server_id: takeoverForm.value.target_server_id,
      port: takeoverForm.value.port || 0,
      canary_server_id: takeoverForm.value.canary_server_id || 0,
    });
    takeoverOpen.value = false;
    startTaskTracking(r.task_id);
  } catch (e) {
    takeoverErr.value = e instanceof Error ? e.message : '接管失败';
  } finally {
    takeoverBusy.value = false;
  }
}

// ================= 使用端凭证打包下载 =================
async function downloadCreds() {
  errorMsg.value = '';
  try {
    await api.downloadWGCreds();
    okMsg.value = '凭证包已开始下载（文件名里的 A=现役 / B=备援）';
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '下载失败';
  }
}

// ================= 资产中转 =================
const assetsPanelOpen = ref(false);
const assetsLoading = ref(false);
const assets = ref<WGAssetItem[]>([]);
const assetProbe = ref<Record<number, WGAssetProbeResult[]>>({});
const assetBusyId = ref(0);
const assetModal = ref<{
  id: number;
  name: string;
  version: string;
  arch: string;
  sha256: string;
  sourcesText: string;
  note: string;
  busy: boolean;
  error: string;
} | null>(null);
const pushAsset = ref<WGAssetItem | null>(null);
const pushServerId = ref(0);
const pushBusy = ref(false);
const pushErr = ref('');

async function loadAssets() {
  assetsLoading.value = true;
  try {
    const r = await api.getWGAssets();
    assets.value = r.assets ?? [];
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '读取资产列表失败';
  } finally {
    assetsLoading.value = false;
  }
}

// 展开时才拉列表（默认折叠，省一次请求）
function toggleAssetsPanel() {
  assetsPanelOpen.value = !assetsPanelOpen.value;
  if (assetsPanelOpen.value && !assets.value.length) void loadAssets();
}

function openAssetModal(as?: WGAssetItem) {
  assetModal.value = {
    id: as?.id || 0,
    name: as?.name || '',
    version: as?.version || '',
    arch: as?.arch || '',
    sha256: as?.sha256 || '',
    sourcesText: (as?.sources || []).join('\n'),
    note: as?.note || '',
    busy: false,
    error: '',
  };
}

async function submitAsset() {
  const m = assetModal.value;
  if (!m) return;
  const sources = m.sourcesText.split('\n').map((s) => s.trim()).filter(Boolean);
  if (!m.name.trim()) {
    m.error = '请填写资产名';
    return;
  }
  if (!sources.length) {
    m.error = '至少填一个下载源（http/https）';
    return;
  }
  m.busy = true;
  m.error = '';
  try {
    await api.upsertWGAsset({
      name: m.name.trim(),
      version: m.version.trim(),
      arch: m.arch.trim(),
      sha256: m.sha256.trim(),
      sources,
      note: m.note.trim(),
    });
    assetModal.value = null;
    await loadAssets();
    okMsg.value = '资产已保存';
  } catch (e) {
    m.error = e instanceof Error ? e.message : '保存失败';
  } finally {
    if (assetModal.value) assetModal.value.busy = false;
  }
}

function shortUrl(u: string): string {
  try {
    const url = new URL(u);
    return `${url.host}${url.pathname.slice(0, 32)}${url.pathname.length > 32 ? '…' : ''}`;
  } catch {
    return u.slice(0, 40);
  }
}

async function probeAsset(as: WGAssetItem) {
  assetBusyId.value = as.id;
  try {
    const r = await api.probeWGAsset(as.id);
    assetProbe.value = { ...assetProbe.value, [as.id]: r.results ?? [] };
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '测速失败';
  } finally {
    assetBusyId.value = 0;
  }
}

async function fetchAsset(as: WGAssetItem) {
  assetBusyId.value = as.id;
  errorMsg.value = '';
  try {
    const r = await api.fetchWGAsset(as.id);
    okMsg.value = `已缓存到面板：${formatBytes(r.size)}（源 ${shortUrl(r.via)}）`;
    await loadAssets();
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '下载失败';
  } finally {
    assetBusyId.value = 0;
  }
}

function openPushAsset(as: WGAssetItem) {
  pushAsset.value = as;
  pushServerId.value = 0;
  pushErr.value = '';
}

async function submitPushAsset() {
  const as = pushAsset.value;
  if (!as || !pushServerId.value) return;
  pushBusy.value = true;
  pushErr.value = '';
  try {
    await api.pushWGAsset(as.id, pushServerId.value);
    pushAsset.value = null;
    okMsg.value = `资产已推送到节点 /usr/local/bin/`;
  } catch (e) {
    pushErr.value = e instanceof Error ? e.message : '推送失败';
  } finally {
    pushBusy.value = false;
  }
}

function askDeleteAsset(as: WGAssetItem) {
  confirmText.value = `确认删除资产「${as.name} ${as.version}」？面板上的缓存文件会一并清理。`;
  confirmAction = async () => {
    try {
      await api.deleteWGAsset(as.id);
      const next = { ...assetProbe.value };
      delete next[as.id];
      assetProbe.value = next;
      await loadAssets();
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '删除失败';
    }
  };
}

onMounted(async () => {
  await loadData();
  idleTimer = setInterval(() => {
    if (document.hidden) return;
    if (overview.value.running_task && !taskModal.value) void loadData();
  }, 5000);
});

onUnmounted(() => {
  if (taskTimer) clearTimeout(taskTimer);
  if (idleTimer) clearInterval(idleTimer);
});
</script>
