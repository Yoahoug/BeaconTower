<template>
  <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3 select-none">
    <!-- 1. 节点运行状态 -->
    <div class="toy-card p-4 flex items-start gap-3 min-w-0">
      <div class="min-w-0 flex-1">
        <div class="text-[11px] font-bold text-toy-muted tracking-wide">节点运行状态</div>
        <div class="mt-1.5 flex items-baseline gap-1.5 flex-wrap">
          <span class="text-2xl font-black font-mono leading-none text-toy-black">{{ summary.online }}</span>
          <span class="text-xs font-bold text-toy-muted whitespace-nowrap">/ {{ summary.total }} 在线</span>
          <span
            class="px-1.5 py-0.5 text-[10px] font-black border border-toy-black rounded whitespace-nowrap"
            :class="summary.offline ? 'bg-toy-pink/15 text-toy-pink' : 'bg-toy-mint/20 text-emerald-800'"
          >
            {{ summary.offline ? `${summary.offline} 台离线` : '全部在线' }}
          </span>
        </div>
        <div class="mt-1.5 text-[10px] font-bold text-toy-muted truncate">
          在线率 {{ onlineRate }}% · {{ stale ? '展示离线快照' : '采集链路正常' }}
        </div>
      </div>
      <div class="kpi-icon bg-toy-mint/25" title="节点运行健康">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="9" stroke="#121212" stroke-width="2" fill="#E6FAF2"/>
          <circle cx="12" cy="12" r="5" stroke="#121212" stroke-width="1.6" fill="#00E599"/>
          <circle cx="12" cy="12" r="1.8" fill="#121212"/>
        </svg>
      </div>
    </div>

    <!-- 2. 全网实时吞吐 -->
    <div class="toy-card p-4 flex items-start gap-3 min-w-0">
      <div class="min-w-0 flex-1">
        <div class="text-[11px] font-bold text-toy-muted tracking-wide">全网实时吞吐</div>
        <div class="mt-1.5 flex items-center gap-x-3 gap-y-1 flex-wrap font-mono text-sm font-black leading-none">
          <span class="inline-flex items-center gap-1 text-sky-700 whitespace-nowrap" title="实时上行速率">
            <span class="text-[11px]">↑</span>{{ formatBps(summary.up_bps) }}
          </span>
          <span class="inline-flex items-center gap-1 text-amber-700 whitespace-nowrap" title="实时下行速率">
            <span class="text-[11px]">↓</span>{{ formatBps(summary.down_bps) }}
          </span>
        </div>
        <div class="mt-1.5 text-[10px] font-bold text-toy-muted truncate">
          双向合计 {{ formatBps(summary.up_bps + summary.down_bps) }}
        </div>
      </div>
      <div class="kpi-icon bg-toy-cyan/20" title="实时吞吐传输">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none">
          <path d="M8 17V7M8 7L5 10M8 7L11 10" stroke="#121212" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
          <path d="M16 7V17M16 17L13 14M16 17L19 14" stroke="#121212" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
          <rect x="14" y="9" width="4" height="4" rx="1" fill="#38BDF8" fill-opacity="0.45"/>
        </svg>
      </div>
    </div>

    <!-- 3. 实时总功耗 -->
    <div class="toy-card p-4 flex items-start gap-3 min-w-0">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-1.5">
          <span class="text-[11px] font-bold text-toy-muted tracking-wide">实时总功耗</span>
          <span
            class="px-1 py-0.5 text-[9px] font-black rounded whitespace-nowrap border"
            :class="summary.measured_count
              ? 'bg-toy-yellow/60 border-toy-black/70 text-toy-black'
              : 'bg-toy-gray/40 border-toy-gray text-toy-muted'"
          >
            {{ summary.measured_count }} 台计量
          </span>
        </div>
        <div class="mt-1.5 flex items-baseline gap-1.5">
          <span class="text-2xl font-black font-mono leading-none text-toy-black">
            {{ summary.measured_count ? summary.watts : '—' }}
          </span>
          <span v-if="summary.measured_count" class="text-xs font-bold text-toy-muted">W</span>
        </div>
        <div class="mt-1.5 text-[10px] font-bold text-toy-muted truncate" title="本月累计用电量与估算电费">
          <template v-if="summary.measured_count">
            本月 {{ formatKwh(summary.month_kwh) }}
            <template v-if="costMeasured"> · ≈¥{{ summary.est_cost_month }}/月</template>
            <template v-else> · 电费未公开</template>
          </template>
          <template v-else>暂无可测机型</template>
        </div>
      </div>
      <div class="kpi-icon bg-toy-yellow/40" title="RAPL 实时功率">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none">
          <rect x="4" y="6" width="14" height="13" rx="2.5" fill="#FFE047" stroke="#121212" stroke-width="2"/>
          <path d="M9 3H13V6H9V3Z" fill="#121212"/>
          <path d="M11 9L8 13H13L10 17" stroke="#121212" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </div>
    </div>

    <!-- 4. 今日消耗流量 -->
    <div class="toy-card p-4 flex items-start gap-3 min-w-0">
      <div class="min-w-0 flex-1">
        <div class="text-[11px] font-bold text-toy-muted tracking-wide">今日消耗流量</div>
        <div class="mt-1.5 flex items-baseline gap-1.5 min-w-0">
          <template v-if="dayMeasured">
            <span class="text-2xl font-black font-mono leading-none text-toy-black whitespace-nowrap">
              {{ formatBytes(summary.day_in_total + summary.day_out_total) }}
            </span>
          </template>
          <template v-else>
            <span class="text-2xl font-black font-mono leading-none text-toy-black">—</span>
          </template>
        </div>
        <div class="mt-1.5 text-[10px] font-bold text-toy-muted truncate">
          <template v-if="dayMeasured">
            ↑{{ formatBytes(summary.day_out_total) }} · ↓{{ formatBytes(summary.day_in_total) }}
          </template>
          <template v-else>按日流量记录累积中</template>
        </div>
      </div>
      <div class="kpi-icon bg-toy-purple/15" title="全网总流量">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none">
          <rect x="4" y="4" width="16" height="7" rx="1.5" fill="#FFFFFF" stroke="#121212" stroke-width="2"/>
          <line x1="7" y1="7.5" x2="11" y2="7.5" stroke="#121212" stroke-width="1.8" stroke-linecap="round"/>
          <circle cx="16" cy="7.5" r="1.2" fill="#00E599"/>
          <rect x="4" y="13" width="16" height="7" rx="1.5" fill="#FFFFFF" stroke="#121212" stroke-width="2"/>
          <line x1="7" y1="16.5" x2="11" y2="16.5" stroke="#121212" stroke-width="1.8" stroke-linecap="round"/>
          <circle cx="16" cy="16.5" r="1.2" fill="#A78BFA"/>
        </svg>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { PublicSummary } from '../../types/backend';
import { formatBytes, formatBps, formatKwh } from '../../services/format';

const props = defineProps<{
  summary: PublicSummary;
  // 是否有节点下发了电费（关闭「公开页电费」时后端不下发 est_cost_month，
  // 不能兜成 ¥0.00 误导）与今日流量（无按日记录时显示「累积中」而非 0 B）
  costMeasured?: boolean;
  dayMeasured?: boolean;
  // SSE 断流/离线快照时不能继续宣称「采集链路正常」
  stale?: boolean;
}>();

const onlineRate = computed(() => {
  const total = props.summary.total;
  if (!total) return 0;
  return Math.round((props.summary.online / total) * 100);
});
</script>

<style scoped>
.kpi-icon {
  @apply w-9 h-9 rounded-lg border-2 border-toy-black flex items-center justify-center shrink-0;
  box-shadow: 2px 2px 0px #121212;
}
</style>
