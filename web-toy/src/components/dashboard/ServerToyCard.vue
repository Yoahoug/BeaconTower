<template>
  <div 
    class="toy-card p-4 transition-all duration-200 relative overflow-hidden"
    :class="[
      server.status === 'offline' ? 'opacity-70 bg-gray-50/90 border-dashed' : 'hover:-translate-y-0.5 hover:shadow-toy-lg',
      isExpanded ? 'ring-2 ring-toy-black' : ''
    ]"
  >
    <!-- 卡片顶栏：头像 + 名称 + 标签 + 在线状态 -->
    <div class="flex items-start justify-between gap-3">
      <div class="flex items-center gap-3 min-w-0">
        <!-- 头像插槽：根据全局设置切换为 [像素机器人] 或 [系统发行版徽标] -->
        <div class="shrink-0 cursor-pointer" @click="$emit('toggle-avatar')" title="点击切换头像风格">
          <PixelBotAvatar 
            v-if="avatarMode === 'bot'"
            :seed="server.name + server.id" 
            :is-online="server.status === 'online'" 
            size="md"
          />
          <OSDistroBadge 
            v-else
            :os-name="server.profile.os" 
            :is-online="server.status === 'online'" 
            size="md"
          />
        </div>

        <!-- 服务器名称与标签 -->
        <div class="min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <h3 class="font-extrabold text-base text-toy-black truncate leading-tight tracking-tight">
              {{ server.name }}
            </h3>
            <!-- 本机节点标记（采集走本地进程，免 SSH） -->
            <span
              v-if="server.is_self"
              class="px-1.5 py-0.5 text-[10px] font-extrabold bg-toy-cyan/60 border border-toy-black rounded text-toy-black"
              title="面板所在机器：采集走本地进程，无需 SSH"
            >
              本机
            </span>
            <!-- 地区邮票贴纸 -->
            <span class="inline-flex items-center gap-1 px-1.5 py-0.5 text-[11px] font-bold bg-toy-yellow/30 border border-toy-black rounded text-toy-black" :title="regionSourceText">
              {{ formatRegionFlag(server.region) }} {{ server.region }}
            </span>
            <!-- 自定义标签 -->
            <span 
              v-for="tag in server.tags" 
              :key="tag"
              class="px-1.5 py-0.5 text-[10px] font-extrabold bg-toy-gray border border-toy-black rounded text-toy-black"
            >
              {{ tag }}
            </span>
          </div>

          <!-- 系统与硬件规格副标题 -->
          <div class="text-[11px] font-bold text-toy-muted flex items-center gap-1.5 mt-0.5 truncate">
            <span>{{ server.profile.os }} {{ server.profile.os_version }}</span>
            <span>·</span>
            <span>{{ server.profile.arch }}</span>
            <span>·</span>
            <span>{{ server.profile.cpu_cores }} Cores</span>
            <span>·</span>
            <span>{{ formatBytes(server.profile.mem_total) }}</span>
            <span v-if="server.profile.virt && server.profile.virt !== 'none'" class="uppercase text-[9px] bg-toy-black/5 px-1 rounded">
              {{ server.profile.virt }}
            </span>
          </div>
        </div>
      </div>

      <!-- 在线/离线状态胶囊 -->
      <div class="shrink-0">
        <span 
          v-if="server.status === 'online'"
          class="toy-badge bg-toy-mint text-toy-black"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-toy-black animate-pulse"></span>
          ONLINE
        </span>
        <span 
          v-else
          class="toy-badge bg-gray-200 text-toy-muted border-gray-400 shadow-none"
        >
          OFFLINE
        </span>
      </div>
    </div>

    <!-- 核心指标条：CPU、内存、磁盘 (高对比度、大字清晰) -->
    <div class="mt-4 space-y-2.5">
      <!-- 1. CPU -->
      <div>
        <div class="flex items-center justify-between text-xs font-black text-toy-black mb-1">
          <span class="tracking-wide">CPU 使用率</span>
          <span class="font-mono text-sm" :class="server.metrics.cpu_pct > 80 ? 'text-toy-pink' : 'text-toy-black'">
            {{ server.status === 'online' ? server.metrics.cpu_pct.toFixed(1) + '%' : '--' }}
          </span>
        </div>
        <div class="toy-groove h-3.5 flex items-center p-0.5 bg-gray-100">
          <div 
            class="h-full rounded transition-all duration-300 border border-toy-black"
            :class="server.metrics.cpu_pct > 80 ? 'bg-toy-pink' : 'bg-toy-yellow'"
            :style="{ width: server.status === 'online' ? `${server.metrics.cpu_pct}%` : '0%' }"
          ></div>
        </div>
      </div>

      <!-- 2. RAM -->
      <div>
        <div class="flex items-center justify-between text-xs font-black text-toy-black mb-1">
          <span class="tracking-wide">内存 RAM</span>
          <span class="font-mono text-xs">
            <template v-if="server.status === 'online'">
              {{ formatBytes(server.metrics.mem_used) }} / {{ formatBytes(server.profile.mem_total) }}
              <span class="text-toy-muted font-bold ml-1">({{ memPercentage }}%)</span>
            </template>
            <template v-else>--</template>
          </span>
        </div>
        <div class="toy-groove h-3.5 flex items-center p-0.5 bg-gray-100">
          <div 
            class="h-full rounded transition-all duration-300 border border-toy-black bg-toy-cyan"
            :style="{ width: server.status === 'online' ? `${memPercentage}%` : '0%' }"
          ></div>
        </div>
      </div>

      <!-- 3. DISK -->
      <div>
        <div class="flex items-center justify-between text-xs font-black text-toy-black mb-1">
          <span class="tracking-wide">存储 磁盘</span>
          <span class="font-mono text-xs">
            <template v-if="server.status === 'online'">
              {{ formatBytes(server.metrics.disk_used) }} / {{ formatBytes(server.profile.disk_total) }}
              <span class="text-toy-muted font-bold ml-1">({{ diskPercentage }}%)</span>
            </template>
            <template v-else>--</template>
          </span>
        </div>
        <div class="toy-groove h-3.5 flex items-center p-0.5 bg-gray-100">
          <div 
            class="h-full rounded transition-all duration-300 border border-toy-black bg-toy-orange"
            :style="{ width: server.status === 'online' ? `${diskPercentage}%` : '0%' }"
          ></div>
        </div>
      </div>
    </div>

    <!-- 实时走势：CPU / 功耗（与 classic 节点卡同口径的 40 点滚动窗口） -->
    <div v-if="server.status === 'online'" class="mt-3.5 p-2.5 rounded-lg bg-toy-bg/60 border-2 border-toy-black/80 space-y-2">
      <div class="flex items-center justify-between text-[10px] font-black text-toy-black">
        <span>CPU 走势 · 近 2 分钟</span>
        <span class="font-mono text-toy-muted">负载 {{ server.metrics.load1.toFixed(2) }} / {{ server.metrics.load5.toFixed(2) }} / {{ server.metrics.load15.toFixed(2) }}</span>
      </div>
      <ToySparkLine
        :data="cpuHistory"
        color="#38BDF8"
        :height="48"
        :format="(v) => `${Math.round(v)}%`"
      />
      <template v-if="hasPower && powerHistory.length > 1">
        <div class="flex items-center justify-between text-[10px] font-black text-toy-black">
          <span>功耗走势 · 与 CPU 同窗</span>
          <span class="font-mono text-toy-muted">{{ server.power?.total_w }} W</span>
        </div>
        <ToySparkLine
          :data="powerHistory"
          color="#FFE047"
          :height="40"
          :format="(v) => `${Number(v).toFixed(1)} W`"
        />
      </template>
    </div>

    <!-- 吞吐与今日流量 -->
    <div class="mt-4 pt-3 border-t-2 border-toy-gray/80 flex items-center justify-between flex-wrap gap-2 text-xs font-extrabold text-toy-black">
      <!-- 实时双向速率 -->
      <div class="flex items-center gap-2 font-mono">
        <span class="inline-flex items-center gap-0.5 text-sky-700" title="实时上传速率">
          <span>↑</span> {{ server.status === 'online' ? formatBps(server.metrics.net_out_bps) : '--' }}
        </span>
        <span class="inline-flex items-center gap-0.5 text-blue-700" title="实时下载速率">
          <span>↓</span> {{ server.status === 'online' ? formatBps(server.metrics.net_in_bps) : '--' }}
        </span>
      </div>

      <!-- 今日收发累计（按日流量记录） -->
      <span
        v-if="server.traffic_today"
        class="inline-flex items-center gap-1 px-1.5 py-0.5 text-[11px] font-mono bg-toy-mint/40 border border-toy-black rounded text-toy-black"
        title="今日收发字节累计（按日流量记录，实时聚合）"
      >
        <span>今日</span>
        <span>↑{{ formatBytes(server.traffic_today.out_total) }}</span>
        <span>·</span>
        <span>↓{{ formatBytes(server.traffic_today.in_total) }}</span>
      </span>

      <div class="flex items-center gap-2">
        <!-- 温度 -->
        <span 
          v-if="server.power?.temp_c != null" 
          class="inline-flex items-center gap-0.5 text-[11px] font-mono text-toy-muted"
          title="CPU 实时温度"
        >
          <svg class="w-3 h-3 text-toy-black" viewBox="0 0 24 24" fill="none">
            <path d="M14 14.76V3.5C14 2.67 13.33 2 12.5 2C11.67 2 11 2.67 11 3.5V14.76C9.8 15.42 9 16.71 9 18.2C9 20.3 10.7 22 12.8 22C14.9 22 16.6 20.3 16.6 18.2C16.6 16.71 15.8 15.42 14.6 14.76H14Z" stroke="#121212" stroke-width="2" fill="#FF5E85"/>
          </svg>
          <span>{{ server.power.temp_c.toFixed(1) }}°C</span>
        </span>

        <!-- 在线运行天数 -->
        <span class="inline-flex items-center gap-0.5 text-[11px] font-mono text-toy-muted" title="在线时长">
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none">
            <circle cx="12" cy="12" r="9" stroke="#121212" stroke-width="2"/>
            <polyline points="12 7 12 12 15 15" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
          </svg>
          <span>{{ formatUptime(server.metrics.uptime_s) }}</span>
        </span>
      </div>
    </div>

    <!-- 采集口径标注：上下行只统计物理网卡，桥/veth/隧道不重复计入 -->
    <div
      v-if="server.status === 'online' && netScopeText"
      class="mt-2 text-[10px] font-bold text-toy-muted truncate"
      title="上下行只统计此网卡：桥（br0/docker0）/veth/隧道（wg0 等）与物理网卡是同一份流量，不重复计入；对比 1panel 等工具的「所有网卡」口径时请注意区分"
    >
      📌 {{ netScopeText }} · 不含桥/veth/隧道
    </div>

    <!-- 功耗条：整机 + CPU 分项 + 频率 + 电量/电费；不可用时说明原因 -->
    <div
      v-if="hasPower"
      class="mt-2.5 px-2.5 py-2 rounded-lg bg-toy-yellow/25 border-2 border-toy-black flex items-center justify-between flex-wrap gap-2"
      :title="`整机 ${server.power?.total_w} W（CPU ${server.power?.cpu_w ?? '—'} W）`"
    >
      <span class="inline-flex items-center gap-1 text-xs font-black text-toy-black">
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
          <path d="M13 2L3 14H12L11 22L21 10H12L13 2Z" fill="#121212" stroke="#121212" stroke-width="1.5" stroke-linejoin="round"/>
        </svg>
        <span class="font-mono text-sm">{{ server.power?.total_w }} W</span>
        <span class="text-[10px] font-bold text-toy-muted font-mono">CPU {{ server.power?.cpu_w ?? '—' }} W</span>
        <span class="text-[10px] font-bold text-toy-muted font-mono">{{ formatGhz(server.power?.freq_mhz) }}</span>
      </span>
      <span class="text-[10px] font-bold text-toy-muted font-mono">
        今日 {{ formatKwh(server.power?.today_kwh) }} · 月 {{ formatKwh(server.power?.month_kwh) }}<template v-if="costPublished"> ≈ {{ formatCost(server.power?.est_cost_month) }}</template>
      </span>
    </div>
    <div
      v-else-if="server.power"
      class="mt-2.5 px-2.5 py-1.5 rounded-lg bg-toy-gray/40 border border-toy-black/40 text-[10px] font-bold text-toy-muted"
      :title="powerNoReason"
    >
      🔌 功耗不可用 · {{ server.status === 'online' ? '未暴露 RAPL 功率计接口' : '节点离线' }}
    </div>

    <!-- 底部连接与更新状态 -->
    <div class="mt-2.5 flex items-center justify-between flex-wrap gap-2 text-[10px] font-bold text-toy-muted">
      <span class="font-mono" title="连接数统计含 TIME_WAIT/CLOSE_WAIT 等全部套接字表项，数值偏大于活跃连接">
        TCP {{ server.metrics.tcp_conns }} · UDP {{ server.metrics.udp_conns }} · 进程 {{ server.metrics.processes }} · {{ server.profile.virt }} · {{ server.profile.cpu_cores }}核
      </span>
      <span
        class="font-mono"
        :title="server.status === 'online' ? '距最近一次采集更新' : '采集失联'"
      >
        {{ server.status === 'online' ? `更新于 ${formatAgo(lastUpdated)}` : '断开 · 采集失联' }}
      </span>
    </div>

    <!-- 展开详情条 -->
    <div class="mt-2.5 flex items-center justify-between pt-2 border-t border-dashed border-toy-gray text-[11px]">
      <span class="text-toy-muted truncate max-w-[240px]" :title="server.note_public">
        {{ server.note_public || '无公开备注' }}
      </span>
      <div class="flex items-center gap-1.5">
        <button 
          @click="$emit('select-detail', server)"
          class="px-2 py-0.5 rounded text-[10px] font-extrabold border border-toy-black bg-toy-yellow hover:bg-yellow-300 transition-colors shadow-toy-sm cursor-pointer"
          title="查看该节点历史趋势曲线"
        >
          历史曲线 ➔
        </button>
        <button 
          @click="isExpanded = !isExpanded"
          class="px-2 py-0.5 rounded text-[10px] font-extrabold border border-toy-black bg-white hover:bg-toy-cyan transition-colors shadow-toy-sm cursor-pointer"
        >
          {{ isExpanded ? '收起 ▴' : '硬件画像 ▾' }}
        </button>
      </div>
    </div>

    <!-- 展开后的深度指标明细 -->
    <div v-if="isExpanded" class="mt-3 p-3 rounded-lg bg-toy-bg border-2 border-toy-black space-y-2 text-xs">
      <div class="font-black text-toy-black border-b border-toy-black/20 pb-1 flex items-center gap-1.5">
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
          <rect x="3" y="3" width="18" height="18" rx="2" stroke="#121212" stroke-width="2"/>
          <line x1="3" y1="9" x2="21" y2="9" stroke="#121212" stroke-width="2"/>
          <line x1="9" y1="21" x2="9" y2="9" stroke="#121212" stroke-width="2"/>
        </svg>
        <span>硬件画像与运行指标</span>
      </div>
      <div class="grid grid-cols-2 gap-2 font-mono text-[11px]">
        <div>系统: {{ server.profile.os }} {{ server.profile.os_version }}</div>
        <div>架构: {{ server.profile.arch }}</div>
        <div>核心: {{ server.profile.cpu_cores }} 核</div>
        <div>虚拟化: {{ server.profile.virt || '—' }}</div>
        <div>内存: {{ formatBytes(server.profile.mem_total) }}</div>
        <div>磁盘: {{ formatBytes(server.profile.disk_total) }}</div>
        <div>交换分区: {{ formatBytes(server.metrics.swap_used) }} 已用</div>
        <div>采集网卡: {{ server.profile.net_ifaces || '未判定' }}</div>
        <div>负载: {{ server.metrics.load1.toFixed(2) }} / {{ server.metrics.load5.toFixed(2) }} / {{ server.metrics.load15.toFixed(2) }}</div>
        <div>活动进程: {{ server.metrics.processes }} 个</div>
        <div>TCP 连接: {{ server.metrics.tcp_conns }} 条</div>
        <div>UDP 连接: {{ server.metrics.udp_conns }} 条</div>
        <div v-if="server.power">月度用电: {{ formatKwh(server.power.month_kwh) }}</div>
        <div v-if="server.power && costPublished">预估电费: {{ formatCost(server.power.est_cost_month) }}</div>
        <div v-if="server.power">供电方式: {{ server.power.power_source === 'battery' ? '电池' : '交流电' }}</div>
        <div v-if="server.traffic_today">今日下行: {{ formatBytes(server.traffic_today.in_total) }}</div>
        <div v-if="server.traffic_today">今日上行: {{ formatBytes(server.traffic_today.out_total) }}</div>
        <div>地区来源: {{ regionSourceText }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { ServerItem } from '../../types/backend';
import { formatBytes, formatBps, formatUptime, formatKwh, formatCost, formatGhz, formatAgo } from '../../services/format';
import PixelBotAvatar from '../icons/PixelBotAvatar.vue';
import OSDistroBadge from '../icons/OSDistroBadge.vue';
import ToySparkLine from '../charts/ToySparkLine.vue';

const props = withDefaults(defineProps<{
  server: ServerItem;
  avatarMode: 'bot' | 'os';
  // 本地滚动窗口（classic 由 monitor store 维护的 40 点 CPU/功耗走势）
  cpuHistory?: number[];
  powerHistory?: number[];
  // 距最近一次采集刷新的秒数（classic agoText(lastUpdated)）
  lastUpdated?: number;
}>(), {
  cpuHistory: () => [],
  powerHistory: () => [],
  lastUpdated: 0,
});

defineEmits<{
  (e: 'toggle-status', id: number): void;
  (e: 'toggle-avatar'): void;
  (e: 'select-detail', server: ServerItem): void;
}>();

const isExpanded = ref(false);

const memPercentage = computed(() => {
  if (!props.server.profile.mem_total) return 0;
  return Math.round((props.server.metrics.mem_used / props.server.profile.mem_total) * 100);
});

const diskPercentage = computed(() => {
  if (!props.server.profile.disk_total) return 0;
  return Math.round((props.server.metrics.disk_used / props.server.profile.disk_total) * 100);
});

// 功耗是否真实可读：RAPL 机型在线才有读数（classic hasPower 同口径）
const hasPower = computed(() => !!props.server.power && props.server.status === 'online');

const powerNoReason = computed(() => {
  if (props.server.status !== 'online') return '离线节点无功耗读数';
  const p = props.server.power;
  if (!p) return '未启用功耗采集';
  if (p.cpu_w == null && !p.total_w) return '该机型未暴露 RAPL 功率计接口（部分 AMD / 虚拟机），无法测量功耗';
  return '该机型未暴露 RAPL 功率计接口';
});

// 电费是否公开：后端关闭「公开页电费」时不下发电费字段，不能兜成 ¥0.00
const costPublished = computed(() => props.server.power?.est_cost_month != null);

const regionSourceText = computed(() =>
  props.server.region_source === 'manual' ? '手动指定' : '自动定位',
);

// 流量口径标注：net_ifaces 为空＝采集侧未能判定（历史数据/老版本探针）
const netScopeText = computed(() => {
  const ifaces = props.server.profile?.net_ifaces;
  if (!ifaces) return '';
  const n = ifaces.split(',').filter(Boolean).length;
  return n > 1 ? `物理网卡 ×${n}` : `网卡 ${ifaces}`;
});

function formatRegionFlag(region: string): string {
  if (!region) return '🌐';
  if (region.includes('JP') || region.includes('日本') || region.includes('东京')) return '🇯🇵';
  if (region.includes('HK') || region.includes('香港')) return '🇭🇰';
  if (region.includes('CN') || region.includes('上海') || region.includes('北京') || region.includes('广州')) return '🇨🇳';
  if (region.includes('SG') || region.includes('新加坡')) return '🇸🇬';
  if (region.includes('DE') || region.includes('法兰克福') || region.includes('德国')) return '🇩🇪';
  if (region.includes('US') || region.includes('硅谷') || region.includes('美国')) return '🇺🇸';
  return '🌐';
}
</script>
