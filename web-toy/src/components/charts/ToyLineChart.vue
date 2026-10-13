<template>
  <div class="space-y-1">
    <div
      ref="areaRef"
      class="relative w-full"
      :style="{ height: `${height}px` }"
      @mousemove="handleMove"
      @mouseleave="hoverIndex = null"
    >
      <svg class="w-full h-full" :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none">
        <!-- Y 轴网格线 + 刻度（0 / 中 / 顶） -->
        <template v-for="g in gridLines" :key="`g-${g.pct}`">
          <line :x1="padL" :y1="g.y" :x2="W - padR" :y2="g.y" stroke="#121212" stroke-opacity="0.12" stroke-dasharray="3 3" />
        </template>

        <!-- 各系列：面积 + 折线（null 断线，不画假线） -->
        <template v-for="(s, si) in series" :key="`s-${si}`">
          <polygon
            v-for="(seg, gi) in segmentsOf(s.data)"
            :key="`a-${si}-${gi}`"
            :points="areaOf(seg.points)"
            :fill="s.color"
            fill-opacity="0.22"
          />
          <polyline
            v-for="(seg, gi) in segmentsOf(s.data)"
            :key="`l-${si}-${gi}`"
            :points="seg.points"
            fill="none"
            :stroke="s.color"
            stroke-width="2.2"
            stroke-linejoin="round"
            stroke-linecap="round"
          />
        </template>

        <!-- Hover 十字线 -->
        <template v-if="hoverX !== null">
          <line :x1="hoverX" y1="0" :x2="hoverX" :y2="H" stroke="#121212" stroke-width="1.2" stroke-dasharray="2 2" />
        </template>
      </svg>

      <!-- Y 轴最大/最小刻度（浮在左侧，不占用绘图宽度） -->
      <div class="absolute left-1 top-0 text-[9px] font-mono font-bold text-toy-muted pointer-events-none">{{ fmtY(yTop) }}</div>
      <div class="absolute left-1 bottom-0 text-[9px] font-mono font-bold text-toy-muted pointer-events-none">{{ fmtY(yBottom) }}</div>

      <!-- 悬浮读数 -->
      <div
        v-if="hoverIndex !== null && tipPos"
        ref="tipRef"
        class="absolute z-10 pointer-events-none px-2 py-1 bg-white border-2 border-toy-black rounded-lg shadow-toy-sm text-[10px] font-mono font-bold whitespace-nowrap"
        :style="{ left: `${tipPos.left}px`, top: `${tipPos.top}px` }"
      >
        <div v-if="hoverLabel" class="text-toy-muted mb-0.5">{{ hoverLabel }}</div>
        <div v-for="(s, si) in series" :key="`t-${si}`" class="flex items-center gap-1">
          <span class="w-2 h-2 rounded-sm border border-toy-black inline-block" :style="{ background: s.color }"></span>
          <span :style="{ color: s.color }">{{ s.name }}</span>
          <span class="text-toy-black">{{ fmtVal(s.data[hoverIndex]) }}</span>
        </div>
      </div>
    </div>

    <!-- X 轴首/中/尾刻度 -->
    <div v-if="labels && labels.length" class="flex items-center justify-between text-[9px] font-mono font-bold text-toy-muted px-1">
      <span>{{ labels[0] }}</span>
      <span v-if="labels.length > 2">{{ labels[Math.floor((labels.length - 1) / 2)] }}</span>
      <span>{{ labels[labels.length - 1] }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue';

// 玩具风多序列折线图（对齐 classic TrendChart 的语义：多序列 / 缺采样断线 /
// 自动域 / 悬停读数），外观沿用硬边黑描边 + 平涂填充
export interface ToySeries {
  name: string;
  color: string;
  data: (number | null)[];
}

const props = withDefaults(defineProps<{
  series: ToySeries[];
  height?: number;
  yMin?: number;
  labels?: string[];
  formatY?: (v: number) => string;
  formatVal?: (v: number | null) => string;
}>(), {
  height: 200,
  yMin: undefined,
  labels: () => [],
  formatY: (v: number) => (Math.abs(v) >= 1000 ? `${Math.round(v / 1000)}k` : `${Math.round(v * 10) / 10}`),
  formatVal: (v: number | null) => (v === null ? '—' : `${Math.round(v * 10) / 10}`),
});

const W = 600;
const H = 160;
const padL = 4;
const padR = 4;
const padY = 10;

const count = computed(() => Math.max(...props.series.map((s) => s.data.length), 0));

// 自动值域（classic 走 echarts 自动域同口径）：最小/最大各留 8% 余量，
// 全平坦时给 ±1 的假想跨度，避免除以零
const bounds = computed(() => {
  const vals: number[] = [];
  for (const s of props.series) {
    for (const v of s.data) if (v !== null && Number.isFinite(v)) vals.push(v);
  }
  if (!vals.length) return { lo: 0, hi: 1 };
  let lo = Math.min(...vals);
  let hi = Math.max(...vals);
  if (props.yMin !== undefined) lo = Math.min(lo, props.yMin);
  if (hi - lo < 1e-9) {
    const pad = Math.abs(hi) > 1 ? Math.abs(hi) * 0.08 : 1;
    lo -= pad;
    hi += pad;
    if (lo < 0 && Math.min(...vals) >= 0) lo = 0;
  } else {
    const pad = (hi - lo) * 0.08;
    lo -= pad;
    hi += pad;
    if (lo < 0 && Math.min(...vals) >= 0) lo = 0;
  }
  return { lo, hi };
});

const yTop = computed(() => bounds.value.hi);
const yBottom = computed(() => bounds.value.lo);

const gridLines = computed(() => [1, 2, 3].map((i) => ({ pct: i / 4, y: padY + (i / 4) * (H - 2 * padY) })));

function xOf(i: number): number {
  const n = Math.max(count.value - 1, 1);
  return padL + (i / n) * (W - padL - padR);
}

function yOf(v: number): number {
  const { lo, hi } = bounds.value;
  const span = Math.max(hi - lo, 1e-9);
  return H - padY - ((v - lo) / span) * (H - 2 * padY);
}

// 缺采样断线：按连续非 null 段拆成独立折线/面积（不跨缺口连线）
function segmentsOf(data: (number | null)[]): { points: string; first: number; last: number }[] {
  const out: { points: string; first: number; last: number }[] = [];
  let run: string[] = [];
  let firstIdx = -1;
  let lastIdx = -1;
  const flush = () => {
    if (run.length) out.push({ points: run.join(' '), first: firstIdx, last: lastIdx });
    run = [];
    firstIdx = -1;
    lastIdx = -1;
  };
  data.forEach((v, i) => {
    if (v === null || !Number.isFinite(v)) {
      flush();
      return;
    }
    if (firstIdx === -1) firstIdx = i;
    lastIdx = i;
    run.push(`${xOf(i).toFixed(1)},${yOf(v).toFixed(1)}`);
  });
  flush();
  return out;
}

function areaOf(points: string): string {
  if (!points) return '';
  const pts = points.split(' ');
  const firstX = pts[0].split(',')[0];
  const lastX = pts[pts.length - 1].split(',')[0];
  return `${firstX},${H - padY} ${points} ${lastX},${H - padY}`;
}

const fmtY = (v: number) => props.formatY(v);
const fmtVal = (v: number | null | undefined) => props.formatVal(v === undefined ? null : v);

// ---- 悬停读数 ----
const areaRef = ref<HTMLElement | null>(null);
const tipRef = ref<HTMLElement | null>(null);
const hoverIndex = ref<number | null>(null);
const tipPos = ref<{ left: number; top: number } | null>(null);
const tipSize = ref({ w: 0, h: 0 });

const hoverX = computed(() => (hoverIndex.value === null ? null : xOf(hoverIndex.value)));
const hoverLabel = computed(() => {
  const i = hoverIndex.value;
  if (i === null || !props.labels || !props.labels.length) return '';
  return props.labels[i] ?? '';
});

watch(hoverIndex, async () => {
  if (hoverIndex.value === null) {
    tipPos.value = null;
    return;
  }
  await nextTick();
  const el = areaRef.value;
  const tip = tipRef.value;
  if (tip) tipSize.value = { w: tip.offsetWidth, h: tip.offsetHeight };
  if (!el) return;
  const w = el.clientWidth;
  const h = el.clientHeight;
  if (!w || !h) return;
  const xPx = (xOf(hoverIndex.value) / W) * w;
  const left = Math.min(Math.max(xPx - tipSize.value.w / 2, 2), Math.max(w - tipSize.value.w - 2, 2));
  const above = Math.min(...props.series.map((s) => {
    const v = s.data[hoverIndex.value!];
    return v === null || !Number.isFinite(v) ? h : (yOf(v as number) / H) * h;
  })) - tipSize.value.h - 8;
  const yPx = above >= 2 ? above : 8;
  tipPos.value = { left, top: yPx };
});

function handleMove(e: MouseEvent) {
  const el = areaRef.value;
  if (!el || !count.value) return;
  const rect = el.getBoundingClientRect();
  if (!rect.width) return;
  const xv = ((e.clientX - rect.left) / rect.width) * W;
  const ratio = Math.max(0, Math.min(1, (xv - padL) / (W - padL - padR)));
  hoverIndex.value = Math.round(ratio * (count.value - 1));
}
</script>
