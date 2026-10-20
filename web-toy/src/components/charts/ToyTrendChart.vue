<template>
  <div class="toy-card p-4 sm:p-5 bg-white relative select-none">
    <!-- 图表顶栏：标题 + 图例 + 模式切换 -->
    <div class="flex items-start justify-between gap-3 flex-wrap">
      <div class="flex items-center gap-2.5 min-w-0">
        <div class="w-8 h-8 rounded-lg bg-toy-yellow/35 border-2 border-toy-black flex items-center justify-center shrink-0">
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none">
            <polyline points="3 17 9 11 13 15 21 7" stroke="#121212" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
            <polyline points="14 7 21 7 21 14" stroke="#121212" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
        <div class="min-w-0">
          <h3 class="text-sm font-black text-toy-black flex items-center gap-2">
            <span>{{ chartMode === 'traffic' ? '全网吞吐趋势' : '全网整机功耗趋势' }}</span>
            <span class="w-1.5 h-1.5 rounded-full bg-toy-mint animate-pulse"></span>
          </h3>
          <p class="text-[10px] font-bold text-toy-muted mt-0.5">
            最近 {{ MAX_POINTS }} 次采集 · 实时滚动更新 · 纵轴 {{ chartMode === 'traffic' ? 'B/s' : 'W' }}
          </p>
        </div>
      </div>

      <div class="flex items-center gap-2.5 flex-wrap">
        <!-- 图例即实时读数，避免和顶部 KPI 重复堆数字 -->
        <div v-if="chartMode === 'traffic'" class="flex items-center gap-3 text-[11px] font-mono font-black">
          <span class="inline-flex items-center gap-1.5 text-sky-700 whitespace-nowrap">
            <span class="w-3 h-[3px] rounded-full bg-sky-600"></span>
            上行 {{ formatBps(latestUp) }}
          </span>
          <span class="inline-flex items-center gap-1.5 text-amber-700 whitespace-nowrap">
            <span class="w-3 h-[3px] rounded-full bg-amber-600"></span>
            下行 {{ formatBps(latestDown) }}
          </span>
        </div>
        <div v-else class="flex items-center gap-1.5 text-[11px] font-mono font-black text-amber-700 whitespace-nowrap">
          <span class="w-3 h-[3px] rounded-full bg-amber-600"></span>
          实时功率 {{ latestWatts.toFixed(1) }} W
        </div>

        <!-- 模式切换 -->
        <div class="flex items-center border-2 border-toy-black rounded-lg overflow-hidden bg-white">
          <button
            @click="chartMode = 'traffic'"
            class="px-2 py-1 text-[10px] font-black transition-colors cursor-pointer"
            :class="chartMode === 'traffic' ? 'bg-toy-black text-white' : 'hover:bg-toy-yellow/40 text-toy-black'"
          >
            网络吞吐
          </button>
          <button
            @click="chartMode = 'power'"
            :disabled="!powerAvailable"
            :title="powerAvailable ? '查看整机功耗走势' : '当前没有可测量功耗的机型'"
            class="px-2 py-1 text-[10px] font-black transition-colors disabled:cursor-not-allowed"
            :class="!powerAvailable ? 'opacity-40 text-toy-muted' : chartMode === 'power' ? 'bg-toy-black text-white' : 'hover:bg-toy-yellow/40 text-toy-black'"
          >
            整机功耗
          </button>
        </div>
      </div>
    </div>

    <!-- 绘图区：Y 轴独立成列（HTML 文本不做非等比拉伸），SVG 只画网格与曲线 -->
    <div class="mt-3 rounded-xl border-2 border-toy-black/80 bg-toy-bg/30 p-2 sm:p-3">
      <div class="flex items-start gap-1.5">
        <div class="w-10 shrink-0 h-36 sm:h-44 flex flex-col justify-between text-right text-[9px] leading-none font-mono font-bold text-toy-muted">
          <span v-for="(t, i) in yTicks" :key="i">{{ axisLabel(t) }}</span>
        </div>

        <div class="flex-1 min-w-0">
          <div
            ref="areaRef"
            class="relative h-36 sm:h-44 overflow-hidden"
            role="img"
            :aria-label="a11yLabel"
            @mousemove="handleMouseMove"
            @mouseleave="hoverIndex = null"
          >
            <svg class="absolute inset-0 w-full h-full" :viewBox="`0 0 ${VB_W} ${VB_H}`" preserveAspectRatio="none" aria-hidden="true">
              <defs>
                <linearGradient id="upFill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stop-color="#38BDF8" stop-opacity="0.10" />
                  <stop offset="100%" stop-color="#38BDF8" stop-opacity="0.005" />
                </linearGradient>
                <linearGradient id="downFill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stop-color="#F59E0B" stop-opacity="0.10" />
                  <stop offset="100%" stop-color="#F59E0B" stop-opacity="0.005" />
                </linearGradient>
                <linearGradient id="powerFill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stop-color="#F59E0B" stop-opacity="0.14" />
                  <stop offset="100%" stop-color="#F59E0B" stop-opacity="0.005" />
                </linearGradient>
              </defs>

              <!-- 横向网格 -->
              <line
                v-for="(t, i) in yTicks"
                :key="`grid-${i}`"
                :x1="PAD_X"
                :y1="yOf(t)"
                :x2="VB_W - PAD_X"
                :y2="yOf(t)"
                :stroke="i === yTicks.length - 1 ? 'rgba(18,18,18,0.22)' : 'rgba(18,18,18,0.10)'"
                :stroke-width="i === yTicks.length - 1 ? 1.2 : 1"
                :stroke-dasharray="i === yTicks.length - 1 ? '0' : '3 4'"
                vector-effect="non-scaling-stroke"
              />
              <!-- 纵向参考线 -->
              <line
                v-for="f in [0.25, 0.5, 0.75]"
                :key="`vgrid-${f}`"
                :x1="PAD_X + f * (VB_W - 2 * PAD_X)"
                :y1="PAD_Y"
                :x2="PAD_X + f * (VB_W - 2 * PAD_X)"
                :y2="VB_H - PAD_Y"
                stroke="rgba(18,18,18,0.06)"
                stroke-width="1"
                stroke-dasharray="3 4"
                vector-effect="non-scaling-stroke"
              />

              <template v-if="chartMode === 'traffic'">
                <path :d="downAreaPath" fill="url(#downFill)" />
                <path :d="upAreaPath" fill="url(#upFill)" />
                <path
                  :d="downPath"
                  fill="none"
                  stroke="#D97706"
                  stroke-width="2.2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  vector-effect="non-scaling-stroke"
                />
                <path
                  :d="upPath"
                  fill="none"
                  stroke="#0284C7"
                  stroke-width="2.2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  vector-effect="non-scaling-stroke"
                />
              </template>
              <template v-else>
                <path :d="powerAreaPath" fill="url(#powerFill)" />
                <path
                  :d="powerPath"
                  fill="none"
                  stroke="#D97706"
                  stroke-width="2.4"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  vector-effect="non-scaling-stroke"
                />
              </template>

              <!-- 悬停竖向指示线 -->
              <line
                v-if="hoverIndex !== null"
                :x1="hoverX"
                :y1="PAD_Y"
                :x2="hoverX"
                :y2="VB_H - PAD_Y"
                stroke="#121212"
                stroke-width="1"
                stroke-dasharray="2 3"
                vector-effect="non-scaling-stroke"
              />
            </svg>

            <!-- 末点圆点用 HTML 定位，避免 SVG 非等比缩放把圆压扁 -->
            <span
              v-if="chartMode === 'traffic' && lastDown"
              class="chart-dot"
              :style="{ left: pctX(lastDown.x), top: pctY(lastDown.y), background: '#D97706' }"
            ></span>
            <span
              v-if="chartMode === 'traffic' && lastUp"
              class="chart-dot"
              :style="{ left: pctX(lastUp.x), top: pctY(lastUp.y), background: '#0284C7' }"
            ></span>
            <span
              v-if="chartMode === 'power' && lastPower"
              class="chart-dot"
              :style="{ left: pctX(lastPower.x), top: pctY(lastPower.y), background: '#D97706' }"
            ></span>

            <!-- 悬停点：吞吐模式两条线各一个，功率模式一个 -->
            <template v-if="hoverIndex !== null">
              <template v-if="chartMode === 'traffic'">
                <span
                  class="chart-dot chart-dot--hover"
                  :style="{ left: pctX(upPoints[hoverIndex].x), top: pctY(upPoints[hoverIndex].y), background: '#0284C7' }"
                ></span>
                <span
                  class="chart-dot chart-dot--hover"
                  :style="{ left: pctX(downPoints[hoverIndex].x), top: pctY(downPoints[hoverIndex].y), background: '#D97706' }"
                ></span>
              </template>
              <span
                v-else
                class="chart-dot chart-dot--hover"
                :style="{ left: pctX(powerPoints[hoverIndex].x), top: pctY(powerPoints[hoverIndex].y), background: '#D97706' }"
              ></span>
            </template>

            <!-- 首帧数据未就绪：不要在窗口里铺 30 个 0 点，否则会画出「贴底后突然拉起」 -->
            <div
              v-if="waiting"
              class="absolute inset-0 flex items-center justify-center text-[10px] font-bold text-toy-muted"
            >
              等待首次采集…
            </div>

            <!-- Tooltip -->
            <div
              v-if="hoverSample && tipPos"
              ref="tipRef"
              class="absolute z-10 pointer-events-none px-2 py-1.5 bg-white border-2 border-toy-black rounded-lg text-[10px] font-mono font-bold whitespace-nowrap"
              :style="{ left: `${tipPos.left}px`, top: `${tipPos.top}px`, boxShadow: '2px 2px 0px #121212' }"
            >
              <div class="text-[9px] text-toy-muted mb-0.5">{{ formatClock(hoverSample.time) }}</div>
              <div v-if="chartMode === 'traffic'" class="space-y-0.5">
                <div class="text-sky-700">↑ {{ formatBps(hoverSample.up) }}</div>
                <div class="text-amber-700">↓ {{ formatBps(hoverSample.down) }}</div>
              </div>
              <div v-else class="text-amber-700">⚡ {{ hoverSample.watts.toFixed(1) }} W</div>
            </div>
          </div>

          <div class="mt-1.5 flex items-center justify-between text-[9px] font-mono font-bold text-toy-muted">
            <span>{{ MAX_POINTS }} 次前</span>
            <span>{{ Math.floor(MAX_POINTS / 2) }} 次前</span>
            <span>现在</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue';
import { formatBps } from '../../services/format';

const props = withDefaults(defineProps<{
  currentUpBps: number;
  currentDownBps: number;
  currentWatts: number;
  // 数据是否已从后端就绪：loading 阶段 summary 还是 0，不能拿来铺满窗口
  ready?: boolean;
  // 全网是否有可测功耗的机型：0 台时「整机功耗」页签只剩一条 0 W 直线
  powerAvailable?: boolean;
}>(), { ready: true, powerAvailable: true });

type ChartMode = 'traffic' | 'power';
const chartMode = ref<ChartMode>('traffic');
// 计量机型从有到无（节点离线/换机）时，别把用户留在一条 0 W 直线上
watch(() => props.powerAvailable, (ok) => {
  if (!ok && chartMode.value === 'power') chartMode.value = 'traffic';
});
const MAX_POINTS = 30;

interface Sample {
  up: number;
  down: number;
  watts: number;
  time: number;
}

const samples = ref<Sample[]>([]);
const hoverIndex = ref<number | null>(null);
const areaRef = ref<HTMLElement | null>(null);
const tipRef = ref<HTMLElement | null>(null);
const areaSize = ref({ w: 0, h: 0 });
const tipSize = ref({ w: 0, h: 0 });

// 首帧用当前值铺满整窗（与 classic monitor store 同口径：不画假波形，只给
// 一个诚实的起始基线），之后每帧推进一格。后端无历史接口，趋势即本地窗口。
// v2.9 修正：必须等 ready（summary 已拿到真实值）再铺，否则 loading 阶段的
// 0 值会占满 29 格，首个真实样本只能在最右侧拉出一根尖峰。
const seeded = ref(false);

function pushSample(up: number, down: number, watts: number) {
  if (!seeded.value) {
    samples.value = Array.from({ length: MAX_POINTS }, () => ({
      up, down, watts, time: Date.now(),
    }));
    seeded.value = true;
    return;
  }
  // 悬停期间追加样本会让窗口左移一格：同步把索引左移，读数不会跳变。
  if (hoverIndex.value !== null) {
    hoverIndex.value = Math.max(0, hoverIndex.value - 1);
  }
  samples.value.push({ up, down, watts, time: Date.now() });
  if (samples.value.length > MAX_POINTS) {
    samples.value.splice(0, samples.value.length - MAX_POINTS);
  }
}

const waiting = computed(() => samples.value.length === 0);

function seedCurrent() {
  seeded.value = false;
  pushSample(props.currentUpBps || 0, props.currentDownBps || 0, props.currentWatts || 0);
}

onMounted(() => {
  if (props.ready) seedCurrent();
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
  () => props.ready,
  (ready) => {
    // 快照/首屏数据到达：整窗换成真实值，清掉 loading 阶段的空窗口
    if (ready) seedCurrent();
  },
);

watch(
  () => [props.currentUpBps, props.currentDownBps, props.currentWatts] as const,
  ([up, down, watts]) => {
    if (!props.ready) return;
    pushSample(up || 0, down || 0, watts || 0);
  },
);

const latestUp = computed(() => props.currentUpBps || 0);
const latestDown = computed(() => props.currentDownBps || 0);
const latestWatts = computed(() => props.currentWatts || 0);

// ---------- 坐标系 ----------
const VB_W = 640;
const VB_H = 200;
const PAD_Y = 6;
// 左右留出末点圆点的半径：PAD_X=2 时末点会被容器的 overflow-hidden 裁成半圆
const PAD_X = 8;

// Y 轴取「好看的步长」，避免出现 0.375 W 这类刻度；4 段刻度保证读图有参照。
function niceStep(raw: number): number {
  if (!Number.isFinite(raw) || raw <= 0) return 1;
  const exp = Math.floor(Math.log10(raw));
  const base = Math.pow(10, exp);
  const f = raw / base;
  // 细分档位：150M 这类「顶到 2 倍」的刻度会把曲线压扁，3/4/6/8 能让峰值
  // 落在绘图区的 70–90%，同时刻度值仍然是 30/60/90 这类好读的数。
  const nf = f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 3 ? 3 : f <= 4 ? 4 : f <= 5 ? 5 : f <= 6 ? 6 : f <= 8 ? 8 : 10;
  return nf * base;
}

function maxValue(): number {
  let m = 0;
  for (const s of samples.value) {
    if (chartMode.value === 'traffic') {
      if (s.up > m) m = s.up;
      if (s.down > m) m = s.down;
    } else if (s.watts > m) {
      m = s.watts;
    }
  }
  return m;
}

// 零流量时给 1 KB/s 的兜底刻度，线贴底而不是把空窗口放大成噪声。
const yMax = computed(() => {
  const m = maxValue();
  if (m <= 0) return chartMode.value === 'traffic' ? 1024 : 1;
  const raw = m * 1.12;
  const step = niceStep(raw / 4);
  return Math.ceil(raw / step) * step;
});

const yTicks = computed(() => {
  const max = yMax.value;
  return [0, 1, 2, 3, 4].map((i) => max - (i * max) / 4);
});

function yOf(v: number): number {
  return VB_H - PAD_Y - (Math.max(0, v) / yMax.value) * (VB_H - 2 * PAD_Y);
}

function xOf(i: number): number {
  const n = Math.max(samples.value.length - 1, 1);
  return PAD_X + (i / n) * (VB_W - 2 * PAD_X);
}

function pctX(x: number): string {
  return `${(x / VB_W) * 100}%`;
}

function pctY(y: number): string {
  return `${(y / VB_H) * 100}%`;
}

// ---------- 曲线 ----------
// Catmull-Rom → 三次贝塞尔：视觉顺滑但不过冲（控制点夹在绘图区内）。
function smoothPath(points: { x: number; y: number }[]): string {
  if (!points.length) return '';
  if (points.length === 1) return `M ${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;
  const clampY = (v: number) => Math.min(VB_H - PAD_Y, Math.max(PAD_Y, v));
  let d = `M ${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[i - 1] || points[i];
    const p1 = points[i];
    const p2 = points[i + 1];
    const p3 = points[i + 2] || p2;
    const c1x = p1.x + (p2.x - p0.x) / 6;
    const c1y = clampY(p1.y + (p2.y - p0.y) / 6);
    const c2x = p2.x - (p3.x - p1.x) / 6;
    const c2y = clampY(p2.y - (p3.y - p1.y) / 6);
    d += ` C ${c1x.toFixed(1)} ${c1y.toFixed(1)}, ${c2x.toFixed(1)} ${c2y.toFixed(1)}, ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`;
  }
  return d;
}

const upPoints = computed(() => samples.value.map((s, i) => ({ x: xOf(i), y: yOf(s.up) })));
const downPoints = computed(() => samples.value.map((s, i) => ({ x: xOf(i), y: yOf(s.down) })));
const powerPoints = computed(() => samples.value.map((s, i) => ({ x: xOf(i), y: yOf(s.watts) })));

const upPath = computed(() => smoothPath(upPoints.value));
const downPath = computed(() => smoothPath(downPoints.value));
const powerPath = computed(() => smoothPath(powerPoints.value));

function withBaseline(line: string): string {
  const pts = samples.value;
  if (!pts.length || !line) return '';
  const firstX = xOf(0);
  const lastX = xOf(pts.length - 1);
  return `${line} L ${lastX.toFixed(1)} ${VB_H - PAD_Y} L ${firstX.toFixed(1)} ${VB_H - PAD_Y} Z`;
}

const upAreaPath = computed(() => withBaseline(upPath.value));
const downAreaPath = computed(() => withBaseline(downPath.value));
const powerAreaPath = computed(() => withBaseline(powerPath.value));

const lastUp = computed(() => upPoints.value[upPoints.value.length - 1] || null);
const lastDown = computed(() => downPoints.value[downPoints.value.length - 1] || null);
const lastPower = computed(() => powerPoints.value[powerPoints.value.length - 1] || null);

// ---------- 悬停 ----------
const hoverSample = computed(() => {
  if (hoverIndex.value === null) return null;
  return samples.value[hoverIndex.value] || null;
});

const hoverPoint = computed(() => {
  const s = hoverSample.value;
  if (!s) return null;
  if (chartMode.value === 'traffic') {
    // 锚在两条曲线中更高的一侧，tooltip 永远不压线
    return upPoints.value[hoverIndex.value!].y <= downPoints.value[hoverIndex.value!].y
      ? upPoints.value[hoverIndex.value!]
      : downPoints.value[hoverIndex.value!];
  }
  return powerPoints.value[hoverIndex.value!] || null;
});

const hoverX = computed(() => {
  const i = hoverIndex.value;
  return i === null ? 0 : xOf(i);
});

const tipPos = ref<{ left: number; top: number } | null>(null);

function measureArea() {
  const el = areaRef.value;
  if (!el) return;
  const r = el.getBoundingClientRect();
  areaSize.value = { w: r.width, h: r.height };
  updateTipPos();
}

function updateTipPos() {
  const area = areaSize.value;
  const pt = hoverPoint.value;
  if (!area.w || !area.h || !pt) {
    tipPos.value = null;
    return;
  }
  const tip = tipSize.value;
  const xPx = (pt.x / VB_W) * area.w;
  const yPx = (pt.y / VB_H) * area.h;
  const left = Math.min(Math.max(xPx - tip.w / 2, 2), Math.max(area.w - tip.w - 2, 2));
  const above = yPx - tip.h - 8;
  const top = above >= 2 ? above : Math.min(yPx + 10, Math.max(area.h - tip.h - 2, 2));
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
  if (!el || samples.value.length === 0) return;
  const rect = el.getBoundingClientRect();
  if (!rect.width) return;
  const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
  hoverIndex.value = Math.round(ratio * (samples.value.length - 1));
}

// ---------- 格式化 ----------
function trimNum(n: number): string {
  if (n >= 10) return String(Math.round(n));
  return String(parseFloat(n.toFixed(1)));
}

function axisLabel(v: number): string {
  if (chartMode.value === 'power') {
    if (v <= 0) return '0 W';
    return `${v >= 100 ? Math.round(v) : parseFloat(v.toFixed(1))} W`;
  }
  if (v <= 0) return '0';
  if (v >= 1e9) return `${trimNum(v / 1e9)}G`;
  if (v >= 1e6) return `${trimNum(v / 1e6)}M`;
  if (v >= 1e3) return `${trimNum(v / 1e3)}K`;
  return `${Math.round(v)}B`;
}

function formatClock(ts: number): string {
  const d = new Date(ts);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

const a11yLabel = computed(() =>
  chartMode.value === 'traffic'
    ? `全网吞吐趋势：上行 ${formatBps(latestUp.value)}，下行 ${formatBps(latestDown.value)}，最近 ${MAX_POINTS} 次采集`
    : `全网功耗趋势：实时功率 ${latestWatts.value.toFixed(1)} W，最近 ${MAX_POINTS} 次采集`,
);
</script>

<style scoped>
.chart-dot {
  position: absolute;
  width: 8px;
  height: 8px;
  border: 2px solid #ffffff;
  border-radius: 9999px;
  box-shadow: 0 0 0 1.5px #121212;
  transform: translate(-50%, -50%);
  pointer-events: none;
}
.chart-dot--hover {
  width: 9px;
  height: 9px;
}
</style>
