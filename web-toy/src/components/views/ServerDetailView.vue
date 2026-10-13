<template>
  <div v-if="server" class="space-y-5 select-none">
    <!-- 顶栏：返回按钮 + 服务器名称 + 状态 -->
    <div class="flex items-center justify-between flex-wrap gap-3">
      <div class="flex items-center gap-3">
        <button
          @click="$emit('back')"
          class="toy-btn px-3 py-1.5 rounded-xl bg-white hover:bg-toy-yellow text-xs font-black flex items-center gap-1.5 cursor-pointer"
        >
          <span>◀</span>
          <span>返回总览</span>
        </button>
        <div class="flex items-center gap-2">
          <PixelBotAvatar :seed="server.name + server.id" :is-online="server.status === 'online'" size="md" />
          <div>
            <div class="flex items-center gap-2 flex-wrap">
              <h2 class="text-lg font-black text-toy-black">{{ server.name }}</h2>
              <span v-if="server.is_self" class="toy-badge bg-toy-cyan/60 text-[10px]">本机</span>
              <span class="toy-badge bg-toy-yellow/40 text-[11px]" :title="server.region_source === 'manual' ? '手动指定' : '自动定位'">{{ server.region || '未定位' }}</span>
              <span
                class="toy-badge"
                :class="server.status === 'online' ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'"
              >
                {{ server.status === 'online' ? '● 在线' : '○ 离线' }}
              </span>
            </div>
            <p class="text-xs font-bold text-toy-muted">{{ server.note_public || '无公开备注' }}</p>
          </div>
        </div>
      </div>

      <!-- 快速数据角标 -->
      <div class="flex items-center gap-2 text-xs font-mono font-bold flex-wrap">
        <span class="px-2.5 py-1 bg-white border-2 border-toy-black rounded-lg shadow-toy-sm">
          运行: {{ formatUptime(server.metrics.uptime_s) }}
        </span>
        <span v-if="server.status === 'online' && server.power" class="px-2.5 py-1 bg-toy-yellow border-2 border-toy-black rounded-lg shadow-toy-sm" :title="`整机 ${server.power.total_w} W（CPU ${server.power.cpu_w ?? '—'} W）`">
          ⚡ {{ server.power.total_w }} W
        </span>
        <span
          class="px-2.5 py-1 border-2 border-toy-black rounded-lg shadow-toy-sm"
          :class="stale ? 'bg-toy-yellow/70' : 'bg-white'"
        >
          {{ stale ? `离线 · ${staleText}` : `更新于 ${formatAgo(secondsSinceUpdate)}` }}
        </span>
      </div>
    </div>

    <!-- 1. 硬件规格与画像积木卡片 -->
    <div class="toy-card p-4 bg-white grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3 text-xs">
      <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
        <span class="text-toy-muted font-bold block text-[10px]">操作系统</span>
        <span class="font-extrabold text-toy-black mt-0.5 block truncate">{{ server.profile.os }} {{ server.profile.os_version }}</span>
      </div>
      <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
        <span class="text-toy-muted font-bold block text-[10px]">CPU 架构 & 核心</span>
        <span class="font-extrabold text-toy-black mt-0.5 block">{{ server.profile.cpu_cores }} 核 · {{ server.profile.arch }}</span>
      </div>
      <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
        <span class="text-toy-muted font-bold block text-[10px]">物理内存容量</span>
        <span class="font-extrabold text-toy-black mt-0.5 block font-mono">{{ formatBytes(server.profile.mem_total) }}</span>
      </div>
      <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
        <span class="text-toy-muted font-bold block text-[10px]">磁盘已用 / 总量</span>
        <span class="font-extrabold text-toy-black mt-0.5 block font-mono">
          {{ formatBytes(server.metrics.disk_used) }} / {{ formatBytes(server.profile.disk_total) }}
        </span>
      </div>
      <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
        <span class="text-toy-muted font-bold block text-[10px]">虚拟化 / 交换分区</span>
        <span class="font-extrabold text-toy-black mt-0.5 block">{{ server.profile.virt || '—' }} · {{ formatBytes(server.metrics.swap_used) }}</span>
      </div>
      <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
        <span class="text-toy-muted font-bold block text-[10px]">负载 / 进程 / 连接</span>
        <span class="font-extrabold text-toy-black mt-0.5 block font-mono">
          {{ server.metrics.load1.toFixed(2) }} · {{ server.metrics.processes }} · {{ server.metrics.tcp_conns }}/{{ server.metrics.udp_conns }}
        </span>
      </div>
    </div>

    <!-- 2. 历史趋势（多维度，对齐 classic 四图 + 每日流量） -->
    <div class="toy-card p-5 bg-white space-y-4">
      <div class="flex items-center justify-between flex-wrap gap-2">
        <div class="flex items-center gap-2">
          <span class="text-lg">📊</span>
          <div>
            <h3 class="text-sm font-black text-toy-black">节点历史曲线</h3>
            <p class="text-[10px] font-bold text-toy-muted">
              {{ rangeHint }} · 缺采样处断线，不画假数据
            </p>
          </div>
        </div>

        <!-- 时间窗口切换（对齐 classic RANGE_META） -->
        <div class="flex items-center border-2 border-toy-black rounded-lg overflow-hidden bg-toy-bg shadow-toy-sm">
          <button
            v-for="r in RANGE_META"
            :key="r.key"
            :title="r.hint"
            @click="range = r.key"
            class="px-2.5 py-1 text-xs font-black transition-colors cursor-pointer"
            :class="range === r.key ? 'bg-toy-yellow text-toy-black' : 'hover:bg-white text-toy-muted'"
          >
            {{ r.label }}
          </button>
        </div>
      </div>

      <!-- 加载 / 错误 / 需登录 / 空数据 四态 -->
      <div v-if="loading" class="space-y-3">
        <div v-for="i in 2" :key="`sk-${i}`" class="h-32 bg-toy-gray/40 rounded-xl animate-pulse"></div>
      </div>
      <div v-else-if="needLogin" class="p-8 text-center space-y-2">
        <span class="text-3xl">🔒</span>
        <h4 class="text-sm font-black text-toy-black">需登录查看</h4>
        <p class="text-xs font-bold text-toy-muted">{{ error || '长期历史（7 天）默认仅管理员可见，登录后返回本页继续浏览。' }}</p>
        <button class="toy-btn px-4 py-2 bg-toy-yellow text-xs font-black rounded-lg mt-1 cursor-pointer" @click="$emit('need-login')">
          前往登录
        </button>
      </div>
      <div v-else-if="error" class="toy-card p-6 bg-red-50 border-2.5 border-red-400 text-center space-y-2">
        <p class="text-xs font-black text-red-700">⚠ {{ error }}</p>
        <button class="toy-btn px-3 py-1.5 bg-white text-xs rounded-lg cursor-pointer" @click="load">重试</button>
      </div>
      <div v-else-if="!points.length" class="p-8 text-center space-y-1">
        <span class="text-3xl">🗂️</span>
        <h4 class="text-sm font-black text-toy-black">暂无历史数据</h4>
        <p class="text-xs font-bold text-toy-muted">该节点在此范围内还没有采样（新纳管或离线较久）。</p>
      </div>

      <template v-else>
        <div class="space-y-1">
          <div class="flex items-center justify-between text-xs font-black text-toy-black">
            <span>CPU 使用率 · {{ rangeLabel }}</span>
            <span class="font-mono text-[11px] text-toy-muted">当前 {{ points[points.length - 1].cpu?.toFixed(1) ?? '—' }}% · 峰值 {{ cpuPeak.toFixed(1) }}%</span>
          </div>
          <ToyLineChart :series="cpuSeries" :labels="xLabels" :height="170" :format-y="(v) => `${Math.round(v)}%`" :format-val="(v) => (v === null ? '—' : `${v.toFixed(1)}%`)" />
        </div>

        <div class="space-y-1">
          <div class="flex items-center justify-between text-xs font-black text-toy-black">
            <span>内存已用 · {{ rangeLabel }}</span>
            <span class="font-mono text-[11px] text-toy-muted">总量 {{ formatBytes(server.profile.mem_total) }}</span>
          </div>
          <ToyLineChart :series="memSeries" :labels="xLabels" :height="150" :y-min="memYMin" :format-y="(v) => `${v.toFixed(1)} GB`" :format-val="(v) => (v === null ? '—' : `${v.toFixed(2)} GB`)" />
        </div>

        <div class="space-y-1">
          <div class="flex items-center justify-between text-xs font-black text-toy-black">
            <span>网络吞吐 · {{ rangeLabel }}</span>
            <span class="font-mono text-[11px] text-toy-muted">{{ netScopeText }}</span>
          </div>
          <ToyLineChart :series="netSeries" :labels="xLabels" :height="150" :format-y="formatBps" :format-val="(v) => (v === null ? '—' : formatBps(v))" />
        </div>

        <div v-if="hasPower" class="space-y-1">
          <div class="flex items-center justify-between text-xs font-black text-toy-black">
            <span>整机功耗 · {{ rangeLabel }}</span>
            <span class="font-mono text-[11px] text-toy-muted">RAPL 功率计</span>
          </div>
          <ToyLineChart :series="powerSeries" :labels="xLabels" :height="130" :format-y="(v) => `${v.toFixed(1)} W`" :format-val="(v) => (v === null ? '—' : `${v.toFixed(1)} W`)" />
        </div>
        <p v-else class="text-[11px] font-bold text-toy-muted">
          🔌 该节点在此范围没有功耗采样（未暴露 RAPL 功率计接口或采集未开启）。
        </p>
      </template>
    </div>

    <!-- 3. 每 日 流量记录（上行 / 下行双序列，独立于 range，固定近 30 天） -->
    <div class="toy-card p-5 bg-white space-y-3">
      <div class="flex items-center justify-between flex-wrap gap-2">
        <h3 class="text-sm font-black text-toy-black">每日流量 · 近 {{ TRAFFIC_DAYS }} 天</h3>
        <div class="flex items-center gap-3 text-[11px] font-mono font-bold">
          <span class="flex items-center gap-1 text-sky-700">
            <span class="w-2.5 h-2.5 rounded-sm bg-toy-cyan border border-toy-black"></span>上行（累计字节）
          </span>
          <span class="flex items-center gap-1 text-amber-800">
            <span class="w-2.5 h-2.5 rounded-sm bg-toy-yellow border border-toy-black"></span>下行
          </span>
        </div>
      </div>
      <div v-if="trafficErr" class="p-3 text-xs font-bold text-toy-muted">
        该节点暂无按日流量记录（记录随采集按天累积，新节点需要等一天）。
      </div>
      <ToyLineChart
        v-else-if="traffic.length"
        :series="trafficSeries"
        :labels="trafficLabels"
        :height="170"
        :format-y="(v) => `${v.toFixed(2)} GB`"
        :format-val="(v) => (v === null ? '—' : `${v.toFixed(2)} GB`)"
      />
      <div v-else class="p-3 text-xs font-bold text-toy-muted">暂无归档数据。</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue';
import type { ServerItem, TrafficPoint } from '../../types/backend';
import { formatBytes, formatBps, formatUptime, formatAgo } from '../../services/format';
import { api, type HistorySeries } from '../../services/api';
import PixelBotAvatar from '../icons/PixelBotAvatar.vue';
import ToyLineChart from '../charts/ToyLineChart.vue';

const props = withDefaults(defineProps<{
  server: ServerItem;
  secondsSinceUpdate?: number;
  stale?: boolean;
  staleText?: string;
}>(), {
  secondsSinceUpdate: 0,
  stale: false,
  staleText: '',
});

defineEmits<{
  (e: 'back'): void;
  (e: 'need-login'): void;
}>();

const RANGE_META = [
  { key: '1h' as const, label: '近 1 小时', hint: '原始采样' },
  { key: '6h' as const, label: '近 6 小时', hint: '原始采样' },
  { key: '24h' as const, label: '近 24 小时', hint: '原始采样' },
  { key: '7d' as const, label: '近 7 天', hint: '小时聚合·或需登录' },
];

const range = ref<'1h' | '6h' | '24h' | '7d'>('1h');
const points = ref<HistorySeries[]>([]);
const loading = ref(false);
const error = ref('');
const needLogin = ref(false);

const rangeLabel = computed(() => RANGE_META.find((r) => r.key === range.value)?.label || '');
const rangeHint = computed(() => RANGE_META.find((r) => r.key === range.value)?.hint || '');

function fmtClock(ts: number): string {
  const d = new Date(ts * 1000);
  const p = (n: number) => String(n).padStart(2, '0');
  if (range.value === '7d') return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:00`;
  return `${p(d.getHours())}:${p(d.getMinutes())}`;
}

const xLabels = computed(() => points.value.map((p) => fmtClock(p.ts)));

const cpuSeries = computed(() => [
  { name: 'CPU', color: '#38BDF8', data: points.value.map((p) => (p.cpu == null ? null : p.cpu)) },
]);
const cpuPeak = computed(() => {
  const vals = points.value.map((p) => p.cpu).filter((v): v is number => v != null);
  return vals.length ? Math.max(...vals) : 0;
});
// 内存：单独成图才看得清波动；已用常在总量一半上下浮动，Y 轴抬到「最小值向下取整留 1G」
const memSeries = computed(() => [
  { name: '内存已用', color: '#A78BFA', data: points.value.map((p) => (p.memUsed == null ? null : p.memUsed / 1024 ** 3)) },
]);
const memYMin = computed(() => {
  const gb = points.value.filter((p) => p.memUsed != null).map((p) => (p.memUsed as number) / 1024 ** 3);
  if (!gb.length) return undefined;
  return Math.max(0, Math.floor(Math.min(...gb)) - 1);
});
const netSeries = computed(() => [
  { name: '上行', color: '#38BDF8', data: points.value.map((p) => (p.netOut == null ? null : p.netOut)) },
  { name: '下行', color: '#FFE047', data: points.value.map((p) => (p.netIn == null ? null : p.netIn)) },
]);
const powerSeries = computed(() => [
  { name: '整机功率', color: '#FF5E85', data: points.value.map((p) => (p.power == null ? null : p.power)) },
]);
const hasPower = computed(() => points.value.some((p) => p.power != null));

const netScopeText = computed(() => {
  const ifaces = props.server.profile?.net_ifaces;
  if (!ifaces) return '上下行只统计采集网卡';
  const n = ifaces.split(',').filter(Boolean).length;
  return n > 1 ? `物理网卡 ×${n} · 不含桥/veth/隧道` : `网卡 ${ifaces} · 不含桥/veth/隧道`;
});

let ctrl: AbortController | null = null;

async function load() {
  loading.value = true;
  error.value = '';
  needLogin.value = false;
  try {
    points.value = await api.getServerHistory(props.server.id, range.value);
  } catch (e) {
    points.value = [];
    const err = e as { code?: string; message?: string };
    if (err?.code === 'UNAUTHORIZED') {
      needLogin.value = true;
      error.value = err.message || '该范围历史需登录后查看';
    } else if (/不存在/.test(err?.message || '')) {
      error.value = '节点不存在或已隐藏';
    } else {
      error.value = err?.message || '历史曲线加载失败';
    }
  } finally {
    loading.value = false;
  }
}

// ---------- 按日流量记录（独立于 range，固定近 30 天） ----------
const TRAFFIC_DAYS = 30;
const traffic = ref<TrafficPoint[]>([]);
const trafficErr = ref(false);

const trafficLabels = computed(() =>
  traffic.value.map((p) => {
    const d = new Date(p.ts * 1000);
    return `${d.getMonth() + 1}/${d.getDate()}`;
  }),
);

const trafficSeries = computed(() => [
  { name: '上行', color: '#38BDF8', data: traffic.value.map((p) => p.out_total / 1024 ** 3) },
  { name: '下行', color: '#FFE047', data: traffic.value.map((p) => p.in_total / 1024 ** 3) },
]);

async function loadTraffic() {
  try {
    const resp = await api.getServerTraffic(props.server.id, TRAFFIC_DAYS);
    traffic.value = Array.isArray(resp?.points) ? resp.points : [];
    trafficErr.value = false;
  } catch {
    // 流量图失败不打断主曲线，静默降级
    traffic.value = [];
    trafficErr.value = true;
  }
}

watch(range, load);
watch(() => props.server?.id, () => {
  points.value = [];
  traffic.value = [];
  void load();
  void loadTraffic();
}, { immediate: true });

onUnmounted(() => {
  ctrl?.abort();
  ctrl = null;
});
</script>
