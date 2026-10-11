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
            <div class="flex items-center gap-2">
              <h2 class="text-lg font-black text-toy-black">{{ server.name }}</h2>
              <span class="toy-badge bg-toy-yellow/40 text-[11px]">{{ server.region }}</span>
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
      <div class="flex items-center gap-2 text-xs font-mono font-bold">
        <span class="px-2.5 py-1 bg-white border-2 border-toy-black rounded-lg shadow-toy-sm">
          运行: {{ formatUptime(server.metrics.uptime_s) }}
        </span>
        <span v-if="server.power" class="px-2.5 py-1 bg-toy-yellow border-2 border-toy-black rounded-lg shadow-toy-sm">
          ⚡ {{ server.power.total_w }} W
        </span>
      </div>
    </div>

    <!-- 1. 硬件规格与画像积木卡片 -->
    <div class="toy-card p-4 bg-white grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
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
        <span class="text-toy-muted font-bold block text-[10px]">存储磁盘空间</span>
        <span class="font-extrabold text-toy-black mt-0.5 block font-mono">{{ formatBytes(server.profile.disk_total) }}</span>
      </div>
    </div>

    <!-- 2. 历史趋势波形图与时间选择 -->
    <div class="toy-card p-5 bg-white space-y-4">
      <div class="flex items-center justify-between flex-wrap gap-2">
        <div class="flex items-center gap-2">
          <span class="text-lg">📊</span>
          <div>
            <h3 class="text-sm font-black text-toy-black">节点资源历史曲线</h3>
            <p class="text-[10px] font-bold text-toy-muted">包含 CPU、内存、网络上/下行历史</p>
          </div>
        </div>

        <!-- 时间窗口切换按钮 (1h / 6h / 24h / 7d) -->
        <div class="flex items-center border-2 border-toy-black rounded-lg overflow-hidden bg-toy-bg shadow-toy-sm">
          <button 
            v-for="r in (['1h', '6h', '24h', '7d'] as const)" 
            :key="r"
            @click="timeRange = r"
            class="px-2.5 py-1 text-xs font-black transition-colors cursor-pointer"
            :class="timeRange === r ? 'bg-toy-yellow text-toy-black' : 'hover:bg-white text-toy-muted'"
          >
            {{ r }}
          </button>
        </div>
      </div>

      <!-- 纯 SVG 历史波形图 -->
      <div class="w-full h-48 bg-toy-bg/50 border-2 border-toy-black rounded-xl p-3 relative overflow-hidden">
        <svg class="w-full h-full overflow-visible" viewBox="0 0 600 160" preserveAspectRatio="none">
          <!-- 网格线 -->
          <line x1="0" y1="40" x2="600" y2="40" stroke="#121212" stroke-opacity="0.1" stroke-dasharray="3 3"/>
          <line x1="0" y1="80" x2="600" y2="80" stroke="#121212" stroke-opacity="0.1" stroke-dasharray="3 3"/>
          <line x1="0" y1="120" x2="600" y2="120" stroke="#121212" stroke-opacity="0.1" stroke-dasharray="3 3"/>

          <!-- CPU 历史填充与线 -->
          <polygon :points="chartPolygonPoints" fill="#FFE047" fill-opacity="0.5" />
          <polyline :points="chartPolylinePoints" fill="none" stroke="#121212" stroke-width="2.5" stroke-linecap="round"/>
        </svg>

        <div class="absolute bottom-2 right-3 text-[10px] font-mono font-bold text-toy-muted">
          当前采样 CPU 平均: {{ server.metrics.cpu_pct.toFixed(1) }}%
        </div>
      </div>
    </div>

    <!-- 3. 近 30 天每日收发流量条形图 -->
    <div class="toy-card p-5 bg-white space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-black text-toy-black">近 30 天每日流量汇总 (GB)</h3>
        <span class="text-xs font-mono font-bold text-toy-muted">每日自动归档</span>
      </div>
      <div class="h-24 flex items-end gap-1.5 pt-4 px-2 bg-toy-bg rounded-xl border-2 border-toy-black overflow-x-auto">
        <div 
          v-for="(bar, idx) in trafficBars" 
          :key="idx" 
          class="flex-1 min-w-[8px] bg-toy-cyan border border-toy-black rounded-t hover:bg-toy-yellow transition-all cursor-pointer relative group"
          :style="{ height: `${bar.height}%` }"
        >
          <!-- 悬停 Tooltip -->
          <span class="hidden group-hover:block absolute -top-7 left-1/2 -translate-x-1/2 px-1.5 py-0.5 bg-toy-black text-white text-[9px] font-mono rounded whitespace-nowrap z-10">
            {{ bar.label }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { ServerItem } from '../../types/backend';
import { formatBytes, formatUptime } from '../../services/mock';
import PixelBotAvatar from '../icons/PixelBotAvatar.vue';

const props = defineProps<{
  server: ServerItem;
}>();

defineEmits<{
  (e: 'back'): void;
}>();

const timeRange = ref<'1h' | '6h' | '24h' | '7d'>('1h');

// 模拟历史采样曲线点
const historyPoints = computed(() => {
  const count = timeRange.value === '1h' ? 24 : 32;
  const base = props.server.metrics.cpu_pct;
  const pts = [];
  for (let i = 0; i <= count; i++) {
    const y = Math.max(10, Math.min(90, base + Math.sin(i / 2.5) * 15 + Math.cos(i) * 5));
    pts.push({ x: (i / count) * 600, y: 160 - (y / 100) * 140 });
  }
  return pts;
});

const chartPolylinePoints = computed(() => historyPoints.value.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));
const chartPolygonPoints = computed(() => `0,160 ${chartPolylinePoints.value} 600,160`);

// 模拟 30 天每日流量条
const trafficBars = computed(() => {
  const bars = [];
  for (let i = 0; i < 28; i++) {
    const val = 15 + Math.sin(i / 2) * 10 + (i % 4) * 8;
    bars.push({
      height: Math.min(100, Math.max(15, val * 2.2)),
      label: `${(val * 1.5).toFixed(1)} GB`,
    });
  }
  return bars;
});
</script>
