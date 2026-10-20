<template>
  <div
    class="toy-card p-4 transition-all duration-200 relative overflow-hidden flex flex-col"
    :class="[
      server.status === 'offline' ? 'bg-gray-50/80 border-dashed' : 'hover:-translate-y-0.5',
      isExpanded ? 'ring-2 ring-toy-black/70' : '',
    ]"
  >
    <!-- 身份与状态 -->
    <div class="flex items-start justify-between gap-2.5">
      <div class="flex items-center gap-2.5 min-w-0">
        <button
          type="button"
          class="shrink-0 cursor-pointer"
          title="点击切换头像风格（像素机器人 / 系统徽标）"
          @click="$emit('toggle-avatar')"
        >
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
        </button>

        <div class="min-w-0">
          <div class="flex items-center gap-1.5 min-w-0">
            <h3 class="text-[15px] font-black text-toy-black truncate leading-tight" :title="server.name">
              {{ server.name }}
            </h3>
            <span
              v-if="server.is_self"
              class="shrink-0 px-1 py-0.5 text-[9px] font-black bg-toy-cyan/40 border border-toy-black/60 rounded"
              title="面板所在机器：采集走本地进程，无需 SSH"
            >
              本机
            </span>
          </div>
          <div class="mt-0.5 text-[10px] font-bold text-toy-muted truncate" :title="`${server.profile.os} ${server.profile.os_version} · ${server.profile.arch} · ${server.profile.cpu_cores} 核 · ${formatBytes(server.profile.mem_total)}`">
            {{ osLine }} · {{ server.profile.cpu_cores }} 核 · {{ formatBytes(server.profile.mem_total) }}
          </div>
        </div>
      </div>

      <div class="shrink-0 flex flex-col items-end gap-1">
        <span
          class="inline-flex items-center gap-1 px-1.5 py-0.5 text-[10px] font-black border-2 rounded"
          :class="server.status === 'online'
            ? 'bg-toy-mint/25 border-toy-black text-emerald-800'
            : 'bg-toy-gray/40 border-toy-gray text-toy-muted'"
        >
          <span
            class="w-1.5 h-1.5 rounded-full"
            :class="server.status === 'online' ? 'bg-toy-mint animate-pulse' : 'bg-gray-400'"
          ></span>
          {{ server.status === 'online' ? 'ONLINE' : 'OFFLINE' }}
        </span>
        <span v-if="server.profile.virt && server.profile.virt !== 'none'" class="text-[9px] font-bold text-toy-muted uppercase">
          {{ server.profile.virt }}
        </span>
      </div>
    </div>

    <!-- 地区与标签：单独一行，不再和名称抢宽度 -->
    <div class="mt-2 flex flex-wrap items-center gap-1">
      <span
        class="inline-flex items-center gap-1 px-1.5 py-0.5 text-[10px] font-bold bg-toy-yellow/25 border border-toy-black/50 rounded text-toy-black"
        :title="regionSourceText"
      >
        {{ formatRegionFlag(server.region) }} {{ server.region }}
      </span>
      <span
        v-for="tag in server.tags"
        :key="tag"
        class="px-1.5 py-0.5 text-[9px] font-bold bg-toy-bg border border-toy-black/40 rounded text-toy-black"
      >
        {{ tag }}
      </span>
    </div>

    <!-- 三条核心指标：标签左、读数右、细进度条（离线节点不铺空进度条） -->
    <div v-if="server.status === 'online'" class="mt-3 space-y-2">
      <div v-for="m in metrics" :key="m.key">
        <div class="flex items-baseline justify-between gap-2 text-[10px]">
          <span class="font-bold text-toy-muted">{{ m.label }}</span>
          <span class="font-mono font-black text-toy-black truncate" :title="m.value">
            {{ server.status === 'online' ? m.value : '--' }}
          </span>
        </div>
        <div class="mt-1 h-2 rounded-full border border-toy-black/60 bg-white overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-300"
            :class="server.status === 'online' ? m.color : 'bg-toy-gray'"
            :style="{ width: server.status === 'online' ? `${m.pct}%` : '0%' }"
          ></div>
        </div>
      </div>
    </div>

    <!-- CPU / 功耗走势：两张迷你图并排，默认视图只占一行 -->
    <div
      v-if="server.status === 'online'"
      class="mt-3 grid gap-2"
      :class="hasPower ? 'grid-cols-2' : 'grid-cols-1'"
    >
      <div class="rounded-lg border-2 border-toy-black/60 bg-toy-bg/40 px-2 pt-1.5 pb-1 min-w-0">
        <div class="flex items-center justify-between text-[9px] font-bold text-toy-muted">
          <span>CPU 走势</span>
          <span class="font-mono truncate" title="1 / 5 / 15 分钟平均负载">L {{ loadText }}</span>
        </div>
        <ToySparkLine
          :data="cpuHistory"
          color="#38BDF8"
          :height="36"
          :min="0"
          :max="100"
          :format="(v) => `${Math.round(v)}%`"
        />
      </div>

      <div v-if="hasPower" class="rounded-lg border-2 border-toy-black/60 bg-toy-bg/40 px-2 pt-1.5 pb-1 min-w-0">
        <div class="flex items-center justify-between text-[9px] font-bold text-toy-muted">
          <span>功耗走势</span>
          <span class="font-mono">{{ server.power?.total_w }} W</span>
        </div>
        <ToySparkLine
          :data="powerHistory"
          color="#FFE047"
          :height="36"
          :format="(v) => `${Number(v).toFixed(1)} W`"
        />
      </div>
    </div>

    <!-- 实时吞吐 / 今日流量 -->
    <div
      v-if="server.status === 'online'"
      class="mt-3 grid grid-cols-3 divide-x-2 divide-toy-gray/80 rounded-lg border-2 border-toy-black/70 bg-white py-1.5"
    >
      <div class="px-2 min-w-0">
        <div class="text-[9px] font-bold text-toy-muted">实时上行</div>
        <div class="text-xs font-black font-mono text-sky-700 truncate" :title="formatBps(server.metrics.net_out_bps)">
          {{ formatBps(server.metrics.net_out_bps) }}
        </div>
      </div>
      <div class="px-2 min-w-0">
        <div class="text-[9px] font-bold text-toy-muted">实时下行</div>
        <div class="text-xs font-black font-mono text-amber-700 truncate" :title="formatBps(server.metrics.net_in_bps)">
          {{ formatBps(server.metrics.net_in_bps) }}
        </div>
      </div>
      <div class="px-2 min-w-0">
        <div class="text-[9px] font-bold text-toy-muted">今日流量</div>
        <div
          class="text-xs font-black font-mono text-toy-black truncate"
          :title="server.traffic_today ? `今日 ↑${formatBytes(server.traffic_today.out_total)} · ↓${formatBytes(server.traffic_today.in_total)}` : '按日流量记录累积中'"
        >
          {{ server.traffic_today ? formatBytes(server.traffic_today.in_total + server.traffic_today.out_total) : '—' }}
        </div>
      </div>
    </div>

    <!-- 功耗条：整机 + CPU 分项 + 频率 + 月度电费 -->
    <div
      v-if="hasPower"
      class="mt-2.5 flex items-center justify-between flex-wrap gap-x-2 gap-y-1 rounded-lg border-2 border-toy-black/70 bg-toy-yellow/20 px-2.5 py-1.5"
      :title="`整机 ${server.power?.total_w} W（CPU ${server.power?.cpu_w ?? '—'} W）`"
    >
      <span class="inline-flex items-center gap-1.5 text-[11px] font-black text-toy-black min-w-0">
        <svg class="w-3.5 h-3.5 shrink-0" viewBox="0 0 24 24" fill="none">
          <path d="M13 2L3 14H12L11 22L21 10H12L13 2Z" fill="#121212" stroke="#121212" stroke-width="1.5" stroke-linejoin="round"/>
        </svg>
        <span class="font-mono text-sm whitespace-nowrap">{{ server.power?.total_w }} W</span>
        <span class="text-[9px] font-bold text-toy-muted font-mono truncate">
          CPU {{ server.power?.cpu_w ?? '—' }} W · {{ formatGhz(server.power?.freq_mhz) }}
        </span>
      </span>
      <span class="text-[9px] font-bold text-toy-muted font-mono whitespace-nowrap shrink-0">
        月 {{ formatKwh(server.power?.month_kwh) }}<template v-if="costPublished"> · ≈{{ formatCost(server.power?.est_cost_month) }}</template>
      </span>
    </div>
    <div
      v-else-if="server.status === 'online'"
      class="mt-2.5 text-[9px] font-bold text-toy-muted"
      :title="powerNoReason"
    >
      功耗不可用 · {{ powerReasonShort }}
    </div>

    <!-- 次要运行信息 -->
    <div v-if="server.status === 'online'" class="mt-2 flex items-center justify-between flex-wrap gap-x-3 gap-y-1 text-[9px] font-bold text-toy-muted">
      <span v-if="server.power?.temp_c != null" class="inline-flex items-center gap-1" title="CPU 实时温度">
        <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none">
          <path d="M14 14.76V3.5C14 2.67 13.33 2 12.5 2C11.67 2 11 2.67 11 3.5V14.76C9.8 15.42 9 16.71 9 18.2C9 20.3 10.7 22 12.8 22C14.9 22 16.6 20.3 16.6 18.2C16.6 16.71 15.8 15.42 14.6 14.76H14Z" stroke="#121212" stroke-width="2" fill="#FF5E85"/>
        </svg>
        {{ server.power.temp_c.toFixed(1) }}°C
      </span>
      <span class="inline-flex items-center gap-1" title="在线时长">
        <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="9" stroke="#121212" stroke-width="2"/>
          <polyline points="12 7 12 12 15 15" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
        </svg>
        在线 {{ formatUptime(server.metrics.uptime_s) }}
      </span>
      <span class="font-mono truncate" :title="`TCP ${server.metrics.tcp_conns} · UDP ${server.metrics.udp_conns} · 进程 ${server.metrics.processes}`">
        TCP {{ server.metrics.tcp_conns }} · 进程 {{ server.metrics.processes }}
      </span>
    </div>

    <!-- 离线态：不再铺满整屏的「--」，只留一句状态 -->
    <div
      v-else
      class="mt-3 rounded-lg border-2 border-dashed border-toy-gray bg-toy-bg/30 px-3 py-3 text-[10px] font-bold text-toy-muted"
    >
      节点离线 · 采集失联，等待恢复
    </div>

    <!-- 备注与操作 -->
    <div class="mt-auto pt-3">
      <div class="pt-2 border-t-2 border-dashed border-toy-gray flex items-center justify-between gap-2">
        <span class="text-[10px] font-bold text-toy-muted truncate" :title="server.note_public">
          {{ server.note_public || '暂无公开备注' }}
        </span>
        <div class="flex items-center gap-1.5 shrink-0">
          <button
            @click="$emit('select-detail', server)"
            class="px-2 py-1 rounded-md text-[10px] font-black border-2 border-toy-black bg-toy-yellow/60 hover:bg-toy-yellow transition-colors cursor-pointer"
            title="查看该节点历史趋势曲线"
          >
            历史曲线
          </button>
          <button
            @click="isExpanded = !isExpanded"
            class="px-2 py-1 rounded-md text-[10px] font-black border-2 border-toy-black bg-white hover:bg-toy-cyan/30 transition-colors cursor-pointer"
          >
            {{ isExpanded ? '收起 ▲' : '详情 ▼' }}
          </button>
        </div>
      </div>

      <!-- 展开后的深度指标 -->
      <div v-if="isExpanded" class="mt-2.5 rounded-lg bg-toy-bg/60 border-2 border-toy-black/70 p-2.5">
        <div class="grid grid-cols-2 gap-x-3 gap-y-0.5 text-[10px] font-mono">
          <div
            v-for="row in detailRows"
            :key="row.label"
            class="flex items-baseline justify-between gap-2 border-b border-dotted border-toy-gray/80 py-0.5 min-w-0"
          >
            <span class="text-toy-muted shrink-0">{{ row.label }}</span>
            <span class="font-bold text-right truncate" :title="row.value">{{ row.value }}</span>
          </div>
        </div>
        <p
          v-if="server.status === 'online' && netScopeText"
          class="mt-2 text-[9px] font-bold text-toy-muted leading-relaxed"
          title="上下行只统计此网卡：桥（br0/docker0）/veth/隧道（wg0 等）与物理网卡是同一份流量，不重复计入；对比 1panel 等工具的「所有网卡」口径时请注意区分"
        >
          📌 {{ netScopeText }} · 不含桥 / veth / 隧道
        </p>
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
  return Math.min(100, Math.round((props.server.metrics.mem_used / props.server.profile.mem_total) * 100));
});

const diskPercentage = computed(() => {
  if (!props.server.profile.disk_total) return 0;
  return Math.min(100, Math.round((props.server.metrics.disk_used / props.server.profile.disk_total) * 100));
});

// 副标题只保留可扫读的发行版名与版本号，括号里的 codename 留给 title 提示，
// 否则 10px 字号在 390px 手机卡片上会被截成 "Debian 12.5 (bookw…"。
const osLine = computed(() =>
  `${props.server.profile.os} ${props.server.profile.os_version}`
    .replace(/\s*\([^)]*\)\s*/g, ' ')
    .replace(/\s+/g, ' ')
    .trim(),
);

const cpuPercentage = computed(() => {
  const v = Number(props.server.metrics.cpu_pct);
  return Number.isFinite(v) ? Math.min(100, Math.max(0, Math.round(v))) : 0;
});

const metrics = computed(() => [
  {
    key: 'cpu',
    label: 'CPU 使用率',
    value: `${props.server.metrics.cpu_pct.toFixed(1)}%`,
    pct: cpuPercentage.value,
    color: props.server.metrics.cpu_pct > 80 ? 'bg-toy-pink' : 'bg-toy-yellow',
  },
  {
    key: 'mem',
    label: '内存 RAM',
    value: `${formatBytes(props.server.metrics.mem_used)} / ${formatBytes(props.server.profile.mem_total)} · ${memPercentage.value}%`,
    pct: memPercentage.value,
    color: 'bg-toy-cyan',
  },
  {
    key: 'disk',
    label: '存储磁盘',
    value: `${formatBytes(props.server.metrics.disk_used)} / ${formatBytes(props.server.profile.disk_total)} · ${diskPercentage.value}%`,
    pct: diskPercentage.value,
    color: 'bg-toy-orange',
  },
]);

const loadText = computed(() =>
  `${props.server.metrics.load1.toFixed(2)} / ${props.server.metrics.load5.toFixed(2)} / ${props.server.metrics.load15.toFixed(2)}`,
);

// 功耗是否真实可读：RAPL 机型在线才有读数（classic hasPower 同口径）
const hasPower = computed(() => !!props.server.power && props.server.status === 'online');

const powerNoReason = computed(() => {
  if (props.server.status !== 'online') return '离线节点无功耗读数';
  const p = props.server.power;
  if (!p) return '未启用功耗采集';
  if (p.cpu_w == null && !p.total_w) return '该机型未暴露 RAPL 功率计接口（部分 AMD / 虚拟机），无法测量功耗';
  return '该机型未暴露 RAPL 功率计接口';
});

const powerReasonShort = computed(() => {
  if (props.server.status !== 'online') return '节点离线';
  if (!props.server.power) return '未启用采集';
  if (props.server.power.cpu_w == null && !props.server.power.total_w) return '机型未暴露 RAPL';
  return '无 RAPL 读数';
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

const detailRows = computed(() => {
  const s = props.server;
  const rows: { label: string; value: string }[] = [
    { label: '系统', value: `${s.profile.os} ${s.profile.os_version}` },
    { label: '架构', value: s.profile.arch || '—' },
    { label: '虚拟化', value: s.profile.virt || '—' },
    { label: '核心', value: `${s.profile.cpu_cores} 核` },
    { label: '内存', value: formatBytes(s.profile.mem_total) },
    { label: '磁盘', value: formatBytes(s.profile.disk_total) },
    { label: '采集网卡', value: s.profile.net_ifaces || '未判定' },
    { label: '交换分区', value: `${formatBytes(s.metrics.swap_used)} 已用` },
    { label: 'TCP / UDP', value: `${s.metrics.tcp_conns} / ${s.metrics.udp_conns}` },
    { label: '活动进程', value: `${s.metrics.processes} 个` },
    { label: '地区来源', value: regionSourceText.value },
    { label: '数据更新', value: formatAgo(props.lastUpdated) },
  ];
  if (s.power) {
    rows.push(
      { label: '月度用电', value: formatKwh(s.power.month_kwh) },
      { label: '供电方式', value: s.power.power_source === 'battery' ? '电池' : '交流电' },
    );
    if (costPublished.value) rows.push({ label: '预估电费', value: formatCost(s.power.est_cost_month) });
  }
  if (s.traffic_today) {
    rows.push(
      { label: '今日上行', value: formatBytes(s.traffic_today.out_total) },
      { label: '今日下行', value: formatBytes(s.traffic_today.in_total) },
    );
  }
  return rows;
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
