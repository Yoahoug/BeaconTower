<template>
  <div class="toy-card p-4 bg-white relative overflow-hidden select-none">
    <!-- 图表顶栏：标题 + 图例 + 切换按钮 -->
    <div class="flex items-center justify-between flex-wrap gap-2 mb-3">
      <div class="flex items-center gap-2">
        <!-- 纯 SVG 趋势波形小图表图标 -->
        <div class="w-7 h-7 rounded-lg bg-toy-yellow/30 border-2 border-toy-black flex items-center justify-center shrink-0">
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none">
            <polyline points="3 17 9 11 13 15 21 7" stroke="#121212" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
            <polyline points="14 7 21 7 21 14" stroke="#121212" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
        <div>
          <h3 class="text-xs sm:text-sm font-black text-toy-black flex items-center gap-1.5">
            <span>{{ chartMode === 'traffic' ? '全网吞吐趋势 · 实时波形' : '全网整机功耗趋势' }}</span>
            <span class="w-2 h-2 rounded-full bg-toy-mint animate-pulse border border-toy-black"></span>
          </h3>
          <p class="text-[10px] font-bold text-toy-muted">近 30 个采样点 · 实时滚动更新</p>
        </div>
      </div>

      <!-- 右侧图例与切换 -->
      <div class="flex items-center gap-3">
        <!-- 图例 -->
        <div v-if="chartMode === 'traffic'" class="flex items-center gap-2.5 text-xs font-mono font-black">
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded bg-toy-cyan border border-toy-black"></span>
            <span class="text-sky-800">上行 ↑ {{ formatBps(latestUp) }}</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded bg-toy-yellow border border-toy-black"></span>
            <span class="text-amber-800">下行 ↓ {{ formatBps(latestDown) }}</span>
          </div>
        </div>
        <div v-else class="flex items-center gap-1.5 text-xs font-mono font-black">
          <span class="w-3 h-3 rounded bg-toy-yellow border border-toy-black"></span>
          <span class="text-amber-800">实时功率 ⚡ {{ latestWatts.toFixed(1) }} W</span>
        </div>

        <!-- 模式切换按钮 -->
        <div class="flex items-center border-2 border-toy-black rounded-lg overflow-hidden bg-toy-bg">
          <button 
            @click="chartMode = 'traffic'"
            class="px-2 py-0.5 text-[11px] font-black transition-colors"
            :class="chartMode === 'traffic' ? 'bg-toy-black text-white' : 'hover:bg-toy-yellow/50 text-toy-black'"
          >
            网络吞吐
          </button>
          <button 
            @click="chartMode = 'power'"
            class="px-2 py-0.5 text-[11px] font-black transition-colors"
            :class="chartMode === 'power' ? 'bg-toy-black text-white' : 'hover:bg-toy-yellow/50 text-toy-black'"
          >
            整机功耗
          </button>
        </div>
      </div>
    </div>

    <!-- SVG 玩具折线波形图区域 -->
    <div 
      class="w-full h-36 sm:h-44 bg-toy-bg/40 border-2 border-toy-black rounded-xl p-2 relative overflow-hidden"
      @mousemove="handleMouseMove"
      @mouseleave="hoverIndex = null"
    >
      <!-- 背景网格刻度线 -->
      <svg class="absolute inset-0 w-full h-full pointer-events-none" xmlns="http://www.w3.org/2000/svg">
        <line x1="0" y1="25%" x2="100%" y2="25%" stroke="#121212" stroke-dasharray="3 3" stroke-opacity="0.12"/>
        <line x1="0" y1="50%" x2="100%" y2="50%" stroke="#121212" stroke-dasharray="3 3" stroke-opacity="0.12"/>
        <line x1="0" y1="75%" x2="100%" y2="75%" stroke="#121212" stroke-dasharray="3 3" stroke-opacity="0.12"/>
      </svg>

      <!-- 动态波形渲染 -->
      <svg 
        class="w-full h-full overflow-visible" 
        viewBox="0 0 600 160" 
        preserveAspectRatio="none"
      >
        <!-- 1. 网络吞吐双曲线 -->
        <template v-if="chartMode === 'traffic'">
          <!-- 下行填充色块与描边 (明黄) -->
          <polygon 
            :points="downAreaPoints" 
            fill="#FFE047" 
            fill-opacity="0.55" 
          />
          <polyline 
            :points="downLinePoints" 
            fill="none" 
            stroke="#121212" 
            stroke-width="2.5" 
            stroke-linejoin="round"
            stroke-linecap="round"
          />

          <!-- 上行填充色块与描边 (电子青蓝) -->
          <polygon 
            :points="upAreaPoints" 
            fill="#38BDF8" 
            fill-opacity="0.65" 
          />
          <polyline 
            :points="upLinePoints" 
            fill="none" 
            stroke="#121212" 
            stroke-width="2.5" 
            stroke-linejoin="round"
            stroke-linecap="round"
          />

          <!-- 最新数据点脉冲圆圈 -->
          <circle 
            v-if="downPoints.length > 0"
            :cx="downPoints[downPoints.length - 1].x" 
            :cy="downPoints[downPoints.length - 1].y" 
            r="4.5" 
            fill="#FFE047" 
            stroke="#121212" 
            stroke-width="2" 
          />
          <circle 
            v-if="upPoints.length > 0"
            :cx="upPoints[upPoints.length - 1].x" 
            :cy="upPoints[upPoints.length - 1].y" 
            r="4.5" 
            fill="#38BDF8" 
            stroke="#121212" 
            stroke-width="2" 
          />
        </template>

        <!-- 2. 整机功耗单曲线 -->
        <template v-else>
          <polygon 
            :points="powerAreaPoints" 
            fill="#FFE047" 
            fill-opacity="0.6" 
          />
          <polyline 
            :points="powerLinePoints" 
            fill="none" 
            stroke="#121212" 
            stroke-width="3" 
            stroke-linejoin="round"
            stroke-linecap="round"
          />
          <circle 
            v-if="powerPoints.length > 0"
            :cx="powerPoints[powerPoints.length - 1].x" 
            :cy="powerPoints[powerPoints.length - 1].y" 
            r="5" 
            fill="#FF5E85" 
            stroke="#121212" 
            stroke-width="2" 
          />
        </template>

        <!-- Hover 十字指示线与悬浮点 -->
        <template v-if="hoverIndex !== null && hoverIndex >= 0 && hoverIndex < dataHistory.length">
          <line 
            :x1="hoverX" 
            y1="0" 
            :x2="hoverX" 
            y2="160" 
            stroke="#121212" 
            stroke-width="1.5" 
            stroke-dasharray="2 2"
          />
        </template>
      </svg>

      <!-- 悬浮 Tooltip 卡片 -->
      <div 
        v-if="hoverIndex !== null && hoverData"
        class="absolute z-10 pointer-events-none px-2 py-1 bg-white border-2 border-toy-black rounded-lg shadow-toy-sm text-[10px] font-mono font-bold transform -translate-x-1/2 -translate-y-full"
        :style="{ left: `${(hoverIndex / (maxPoints - 1)) * 100}%`, top: '40px' }"
      >
        <div v-if="chartMode === 'traffic'" class="space-y-0.5">
          <div class="text-sky-700">↑ {{ formatBps(hoverData.up) }}</div>
          <div class="text-amber-800">↓ {{ formatBps(hoverData.down) }}</div>
        </div>
        <div v-else class="text-amber-800">
          ⚡ {{ hoverData.watts.toFixed(1) }} W
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue';
import { formatBps } from '../../services/format';

const props = defineProps<{
  currentUpBps: number;
  currentDownBps: number;
  currentWatts: number;
}>();

const chartMode = ref<'traffic' | 'power'>('traffic');
const maxPoints = 30;

interface PointData {
  up: number;
  down: number;
  watts: number;
  time: number;
}

const dataHistory = ref<PointData[]>([]);
const hoverIndex = ref<number | null>(null);

// 初始化一些平滑波动的初始历史点
onMounted(() => {
  const now = Date.now();
  for (let i = maxPoints; i > 0; i--) {
    const factor = 0.8 + Math.sin(i / 3) * 0.3;
    dataHistory.value.push({
      up: Math.max(200 * 1024, (props.currentUpBps || 1.2 * 1024 * 1024) * factor),
      down: Math.max(500 * 1024, (props.currentDownBps || 5.8 * 1024 * 1024) * factor),
      watts: Math.max(30, (props.currentWatts || 68.4) * (0.95 + Math.cos(i / 4) * 0.1)),
      time: now - i * 2000,
    });
  }
});

// 监听外界数据变化，推入新点
watch(
  () => [props.currentUpBps, props.currentDownBps, props.currentWatts],
  ([newUp, newDown, newW]) => {
    dataHistory.value.push({
      up: newUp,
      down: newDown,
      watts: newW,
      time: Date.now(),
    });
    if (dataHistory.value.length > maxPoints) {
      dataHistory.value.shift();
    }
  }
);

const latestUp = computed(() => props.currentUpBps || 0);
const latestDown = computed(() => props.currentDownBps || 0);
const latestWatts = computed(() => props.currentWatts || 0);

// 计算坐标点
const width = 600;
const height = 160;
const paddingY = 15;

// 最大值自动缩放（带保底）
const maxTraffic = computed(() => {
  let m = 1024 * 1024;
  for (const p of dataHistory.value) {
    if (p.up > m) m = p.up;
    if (p.down > m) m = p.down;
  }
  return m * 1.15;
});

const maxPower = computed(() => {
  let m = 50;
  for (const p of dataHistory.value) {
    if (p.watts > m) m = p.watts;
  }
  return m * 1.15;
});

const upPoints = computed(() => {
  return dataHistory.value.map((p, i) => {
    const x = (i / (maxPoints - 1)) * width;
    const y = height - paddingY - (p.up / maxTraffic.value) * (height - 2 * paddingY);
    return { x, y };
  });
});

const downPoints = computed(() => {
  return dataHistory.value.map((p, i) => {
    const x = (i / (maxPoints - 1)) * width;
    const y = height - paddingY - (p.down / maxTraffic.value) * (height - 2 * paddingY);
    return { x, y };
  });
});

const powerPoints = computed(() => {
  return dataHistory.value.map((p, i) => {
    const x = (i / (maxPoints - 1)) * width;
    const y = height - paddingY - (p.watts / maxPower.value) * (height - 2 * paddingY);
    return { x, y };
  });
});

const upLinePoints = computed(() => upPoints.value.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));
const downLinePoints = computed(() => downPoints.value.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));
const powerLinePoints = computed(() => powerPoints.value.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));

const upAreaPoints = computed(() => {
  if (upPoints.value.length === 0) return '';
  return `0,${height} ${upLinePoints.value} ${width},${height}`;
});

const downAreaPoints = computed(() => {
  if (downPoints.value.length === 0) return '';
  return `0,${height} ${downLinePoints.value} ${width},${height}`;
});

const powerAreaPoints = computed(() => {
  if (powerPoints.value.length === 0) return '';
  return `0,${height} ${powerLinePoints.value} ${width},${height}`;
});

const hoverX = computed(() => {
  if (hoverIndex.value === null) return 0;
  return (hoverIndex.value / (maxPoints - 1)) * width;
});

const hoverData = computed(() => {
  if (hoverIndex.value === null) return null;
  return dataHistory.value[hoverIndex.value] || null;
});

function handleMouseMove(e: MouseEvent) {
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
  hoverIndex.value = Math.round(ratio * (maxPoints - 1));
}
</script>
