<template>
  <div>
    <div
      ref="areaRef"
      class="relative w-full"
      :style="{ height: `${height}px` }"
      @mousemove="handleMove"
      @mouseleave="hoverIndex = null"
    >
      <svg class="w-full h-full overflow-visible" :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none">
        <!-- 基线 -->
        <line :x1="padX" :y1="H - padY" :x2="W - padX" :y2="H - padY" stroke="#121212" stroke-opacity="0.18" stroke-width="1.5" stroke-dasharray="3 3" />
        <!-- 面积 + 折线 -->
        <polygon v-if="areaPoints" :points="areaPoints" :fill="color" fill-opacity="0.25" />
        <polyline v-if="linePoints" :points="linePoints" fill="none" stroke="#121212" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
        <!-- 末点 -->
        <circle v-if="lastPoint" :cx="lastPoint.x" :cy="lastPoint.y" r="3.2" :fill="color" stroke="#121212" stroke-width="1.6" />
        <!-- Hover 指示 -->
        <template v-if="hoverPoint">
          <line :x1="hoverPoint.x" y1="0" :x2="hoverPoint.x" :y2="H" stroke="#121212" stroke-width="1.2" stroke-dasharray="2 2" />
          <circle :cx="hoverPoint.x" :cy="hoverPoint.y" r="3" fill="#FFFFFF" stroke="#121212" stroke-width="1.5" />
        </template>
      </svg>

      <div
        v-if="hoverPoint && tipPos"
        class="absolute z-10 pointer-events-none px-1.5 py-0.5 bg-white border-2 border-toy-black rounded-md text-[10px] font-mono font-bold whitespace-nowrap"
        :style="{ left: `${tipPos.left}px`, top: `${tipPos.top}px` }"
      >
        {{ formatted(hoverValue) }}
      </div>

      <div v-if="!data.length" class="absolute inset-0 flex items-center justify-center text-[10px] font-bold text-toy-muted">
        暂无数据
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue';

// 玩具风迷你走势图（对齐 classic SparkLine：40 点滚动窗口 + 悬停读数）
const props = withDefaults(defineProps<{
  data: number[];
  color?: string;
  height?: number;
  format?: (v: number) => string;
}>(), {
  color: '#38BDF8',
  height: 56,
  format: (v: number) => String(Math.round(v)),
});

const W = 300;
const H = 80;
const padX = 4;
const padY = 8;

const min = computed(() => (props.data.length ? Math.min(...props.data) : 0));
const max = computed(() => (props.data.length ? Math.max(...props.data) : 1));

const points = computed(() => {
  const n = props.data.length;
  if (!n) return [] as { x: number; y: number }[];
  const span = Math.max(max.value - min.value, 1e-6);
  return props.data.map((v, i) => ({
    x: padX + (n === 1 ? 0 : (i / (n - 1)) * (W - 2 * padX)),
    y: H - padY - ((v - min.value) / span) * (H - 2 * padY),
  }));
});

const linePoints = computed(() => points.value.map((p) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));
const areaPoints = computed(() => {
  const pts = points.value;
  if (!pts.length) return '';
  return `${pts[0].x},${H - padY} ${linePoints.value} ${pts[pts.length - 1].x},${H - padY}`;
});
const lastPoint = computed(() => (points.value.length ? points.value[points.value.length - 1] : null));

function formatted(v: number | null): string {
  if (v === null) return '—';
  return props.format(v);
}

// ---- 悬停读数 ----
const areaRef = ref<HTMLElement | null>(null);
const hoverIndex = ref<number | null>(null);
const tipPos = ref<{ left: number; top: number } | null>(null);
const tipW = ref(0);

const hoverPoint = computed(() => (hoverIndex.value === null ? null : points.value[hoverIndex.value] || null));
const hoverValue = computed(() => (hoverIndex.value === null ? null : props.data[hoverIndex.value] ?? null));

watch(hoverIndex, async () => {
  if (hoverIndex.value === null) {
    tipPos.value = null;
    return;
  }
  await nextTick();
  const el = areaRef.value;
  const pt = hoverPoint.value;
  if (!el || !pt) return;
  const w = el.clientWidth;
  const h = el.clientHeight;
  const xPx = (pt.x / W) * w;
  const yPx = (pt.y / H) * h;
  const left = Math.min(Math.max(xPx - tipW.value / 2, 2), Math.max(w - tipW.value - 2, 2));
  const above = yPx - 22;
  tipPos.value = { left, top: above >= 2 ? above : Math.min(yPx + 6, Math.max(h - 22, 2)) };
});

function handleMove(e: MouseEvent) {
  const el = areaRef.value;
  if (!el || !props.data.length) return;
  const rect = el.getBoundingClientRect();
  if (!rect.width) return;
  const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
  hoverIndex.value = Math.round(ratio * (props.data.length - 1));
  // 指示卡宽度按内容估算（值字符串长度），避免引入额外测量时机
  tipW.value = Math.max(34, formatted(props.data[hoverIndex.value] ?? 0).length * 6 + 12);
}
</script>
