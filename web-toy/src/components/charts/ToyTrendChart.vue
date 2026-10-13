<template>
  <div class="toy-card p-4 bg-white relative select-none">
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
          <p class="text-[10px] font-bold text-toy-muted">
            近 {{ maxPoints }} 个采样点 · 实时滚动更新
          </p>
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
      class="w-full h-36 sm:h-44 bg-toy-bg/40 border-2 border-toy-black rounded-xl relative overflow-hidden"
      @mousemove="handleMouseMove"
      @mouseleave="hoverIndex = null"
    >
      <!-- 背景网格刻度线 -->
      <svg class="absolute inset-0 w-full h-full pointer-events-none" xmlns="http://www.w3.org/2000/svg">
        <line x1="0" y1="25%" x2="100%" y2="25%" stroke="#121212" stroke-dasharray="3 3" stroke-opacity="0.12"/>
        <line x1="0" y1="50%" x2="100%" y2="50%" stroke="#121212" stroke-dasharray="3 3" stroke-opacity="0.12"/>
        <line x1="0" y1="75%" x2="100%" y2="75%" stroke="#121212" stroke-dasharray="3 3" stroke-opacity="0.12"/>
      </svg>

      <!-- 绘图区（与容器内边距对齐，坐标同时用于 tooltip 定位） -->
      <div ref="areaRef" class="relative w-full h-full">
        <svg 
          class="w-full h-full" 
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
          <template v-if="hoverPoint">
            <line 
              :x1="hoverPoint.x" 
              y1="0" 
              :x2="hoverPoint.x" 
              y2="160" 
              stroke="#121212" 
              stroke-width="1.5" 
              stroke-dasharray="2 2"
            />
            <circle
              :cx="hoverPoint.x"
              :cy="hoverPoint.y"
              r="4"
              fill="#FFFFFF"
              stroke="#121212"
              stroke-width="2"
            />
          </template>
        </svg>

        <!-- 悬浮 Tooltip 卡片（按数据点定位 + 边界内收，绝不被裁切） -->
        <div 
          v-if="hoverData && tipPos"
          ref="tipRef"
          class="absolute z-10 pointer-events-none px-2 py-1 bg-white border-2 border-toy-black rounded-lg shadow-toy-sm text-[10px] font-mono font-bold whitespace-nowrap"
          :style="{ left: `${tipPos.left}px`, top: `${tipPos.top}px` }"
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue';
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
const areaRef = ref<HTMLElement | null>(null);
const tipRef = ref<HTMLElement | null>(null);
// 绘图区像素尺寸（viewBox 600×160 → 像素换算 + tooltip 边界收口用）
const areaSize = ref({ w: 0, h: 0 });
const tipSize = ref({ w: 0, h: 0 });

// 滚动窗口：与 classic monitor store 同口径——首个样本到达时用真实值铺满整窗
//（不是随时间淡入的假波形），之后每帧推进一格。后端无历史接口，趋势即本地窗口。
let seeded = false;

function pushSample(up: number, down: number, watts: number) {
  if (!seeded) {
    // 首帧只有当前值可依据：整窗填同值（classic `Array(HISTORY_LEN).fill(total)`）
    const flat: PointData[] = Array.from({ length: maxPoints }, () => ({
      up, down, watts, time: Date.now(),
    }));
    dataHistory.value = flat;
    seeded = true;
    return;
  }
  dataHistory.value.push({ up, down, watts, time: Date.now() });
  if (dataHistory.value.length > maxPoints) {
    dataHistory.value.splice(0, dataHistory.value.length - maxPoints);
  }
}

onMounted(() => {
  // 父级数据通常先于本组件就绪；先用传入值铺底，父级仍未拉到数据则等首个样本
  if (props.currentUpBps || props.currentDownBps || props.currentWatts) {
    pushSample(props.currentUpBps || 0, props.currentDownBps || 0, props.currentWatts || 0);
  }
  measureArea();
  if (typeof ResizeObserver !== 'undefined' && areaRef.value) {
    ro = new ResizeObserver(measureArea);
    ro.observe(areaRef.value);
  }
});

let ro: ResizeObserver | null = null;
onBeforeUnmount(() => {
  ro?.disconnect();
  ro = null;
});

watch(
  () => [props.currentUpBps, props.currentDownBps, props.currentWatts] as const,
  ([newUp, newDown, newW]) => {
    pushSample(newUp || 0, newDown || 0, newW || 0);
  }
);

const latestUp = computed(() => props.currentUpBps || 0);
const latestDown = computed(() => props.currentDownBps || 0);
const latestWatts = computed(() => props.currentWatts || 0);

// 计算坐标点
const width = 600;
const height = 160;
const paddingY = 15;
// 左右留出圆点半径，避免末点脉冲圈被容器圆角裁掉（截图中的「切角」）
const paddingX = 6;

// 最大值自动缩放：贴住真实峰值（classic 走 echarts 自动域，同理），
// 保底仅用于「全窗口为 0/极小」时不至于除以零——保底过大会把低速流量压成贴底直线
const maxTraffic = computed(() => {
  let m = 0;
  for (const p of dataHistory.value) {
    if (p.up > m) m = p.up;
    if (p.down > m) m = p.down;
  }
  return Math.max(m * 1.15, 16 * 1024);
});

const maxPower = computed(() => {
  let m = 0;
  for (const p of dataHistory.value) {
    if (p.watts > m) m = p.watts;
  }
  return Math.max(m * 1.15, 1);
});

// 横坐标按实际点数铺开；点数不足时按窗口宽度均匀分布（与 classic 一致：
// 首个样本即铺满整宽，随后滚动推进）
function xOf(i: number): number {
  const n = Math.max(dataHistory.value.length - 1, 1);
  return paddingX + (i / n) * (width - 2 * paddingX);
}

const upPoints = computed(() => {
  return dataHistory.value.map((p, i) => ({
    x: xOf(i),
    y: height - paddingY - (p.up / maxTraffic.value) * (height - 2 * paddingY),
  }));
});

const downPoints = computed(() => {
  return dataHistory.value.map((p, i) => ({
    x: xOf(i),
    y: height - paddingY - (p.down / maxTraffic.value) * (height - 2 * paddingY),
  }));
});

const powerPoints = computed(() => {
  return dataHistory.value.map((p, i) => ({
    x: xOf(i),
    y: height - paddingY - (p.watts / maxPower.value) * (height - 2 * paddingY),
  }));
});

const upLinePoints = computed(() => upPoints.value.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));
const downLinePoints = computed(() => downPoints.value.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));
const powerLinePoints = computed(() => powerPoints.value.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));

function areaOf(points: { x: number; y: number }[], linePoints: string): string {
  if (points.length === 0) return '';
  return `${points[0].x},${height} ${linePoints} ${points[points.length - 1].x},${height}`;
}

const upAreaPoints = computed(() => areaOf(upPoints.value, upLinePoints.value));
const downAreaPoints = computed(() => areaOf(downPoints.value, downLinePoints.value));
const powerAreaPoints = computed(() => areaOf(powerPoints.value, powerLinePoints.value));

const hoverData = computed(() => {
  if (hoverIndex.value === null) return null;
  return dataHistory.value[hoverIndex.value] || null;
});

const hoverPoint = computed(() => {
  const i = hoverIndex.value;
  if (i === null || !hoverData.value) return null;
  if (chartMode.value === 'traffic') {
    // 吞吐模式取下行曲线点（视觉主体），保证指示线落在曲线上
    return downPoints.value[i] || upPoints.value[i] || null;
  }
  return powerPoints.value[i] || null;
});

function measureArea() {
  const el = areaRef.value;
  if (!el) return;
  const r = el.getBoundingClientRect();
  areaSize.value = { w: r.width, h: r.height };
  updateTipPos();
}

// Tooltip 像素定位：横向夹在绘图区内（不再被 overflow 裁切、不再挤成多行），
// 纵向优先贴数据点上方，顶部空间不足则翻到点下方
const tipPos = ref<{ left: number; top: number } | null>(null);

function updateTipPos() {
  const area = areaSize.value;
  const pt = hoverPoint.value;
  if (!area.w || !area.h || !pt) {
    tipPos.value = null;
    return;
  }
  const tip = tipSize.value;
  const xPx = (pt.x / width) * area.w;
  const yPx = (pt.y / height) * area.h;
  const left = Math.min(Math.max(xPx - tip.w / 2, 2), Math.max(area.w - tip.w - 2, 2));
  const above = yPx - tip.h - 6;
  const top = above >= 2 ? above : Math.min(yPx + 8, Math.max(area.h - tip.h - 2, 2));
  tipPos.value = { left, top };
}

watch([hoverIndex, chartMode], async () => {
  if (hoverIndex.value === null) {
    tipPos.value = null;
    return;
  }
  await nextTick();
  const el = tipRef.value;
  if (el) tipSize.value = { w: el.offsetWidth, h: el.offsetHeight };
  updateTipPos();
});

function handleMouseMove(e: MouseEvent) {
  const el = areaRef.value;
  if (!el || dataHistory.value.length === 0) return;
  const rect = el.getBoundingClientRect();
  if (!rect.width) return;
  const xv = ((e.clientX - rect.left) / rect.width) * width;
  const ratio = Math.max(0, Math.min(1, (xv - paddingX) / (width - 2 * paddingX)));
  hoverIndex.value = Math.round(ratio * (dataHistory.value.length - 1));
}
</script>
