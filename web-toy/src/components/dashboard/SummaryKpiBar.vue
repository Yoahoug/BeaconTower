<template>
  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 select-none">
    <!-- 1. 在线节点统计 -->
    <div 
      class="toy-card p-3.5 bg-toy-mint/25 flex items-center justify-between transition-transform hover:-translate-y-0.5"
      :style="{ boxShadow: '3.5px 3.5px 0px #121212' }"
    >
      <div>
        <span class="text-xs font-black text-toy-black uppercase tracking-wider block">节点运行状态</span>
        <div class="flex items-baseline gap-2 mt-1">
          <span class="text-2xl font-black font-mono text-toy-black">
            {{ summary.online }} <span class="text-sm font-bold text-toy-muted">/ {{ summary.total }}</span>
          </span>
          <span class="text-xs font-extrabold px-1.5 py-0.2 bg-toy-mint border border-toy-black rounded">
            正常
          </span>
        </div>
      </div>
      <!-- 纯 SVG 玩具雷达指示灯 -->
      <div 
        class="w-10 h-10 rounded-xl bg-white border-2 border-toy-black flex items-center justify-center shrink-0 shadow-toy-sm"
        title="节点运行健康"
      >
        <svg class="w-6 h-6" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="9" stroke="#121212" stroke-width="2.2" fill="#E6FAF2"/>
          <circle cx="12" cy="12" r="5" stroke="#121212" stroke-width="1.8" fill="#00E599"/>
          <circle cx="12" cy="12" r="2" fill="#121212"/>
          <circle cx="10.5" cy="10.5" r="1" fill="#FFFFFF"/>
        </svg>
      </div>
    </div>

    <!-- 2. 全网实时吞吐 -->
    <div 
      class="toy-card p-3.5 bg-toy-cyan/20 flex items-center justify-between transition-transform hover:-translate-y-0.5"
      :style="{ boxShadow: '3.5px 3.5px 0px #121212' }"
    >
      <div>
        <span class="text-xs font-black text-toy-black uppercase tracking-wider block">全网实时吞吐</span>
        <div class="flex items-center gap-3 mt-1 font-mono text-sm font-extrabold text-toy-black">
          <span class="inline-flex items-center text-sky-800">
            <span class="text-xs mr-0.5">↑</span>{{ formatBps(summary.up_bps) }}
          </span>
          <span class="inline-flex items-center text-blue-900">
            <span class="text-xs mr-0.5">↓</span>{{ formatBps(summary.down_bps) }}
          </span>
        </div>
      </div>
      <!-- 纯 SVG 玩具双向网络箭头 -->
      <div 
        class="w-10 h-10 rounded-xl bg-white border-2 border-toy-black flex items-center justify-center shrink-0 shadow-toy-sm"
        title="实时吞吐传输"
      >
        <svg class="w-6 h-6" viewBox="0 0 24 24" fill="none">
          <!-- 上行箭头 (左) -->
          <path d="M8 17V7M8 7L5 10M8 7L11 10" stroke="#121212" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
          <!-- 下行箭头 (右) 填青蓝 -->
          <path d="M16 7V17M16 17L13 14M16 17L19 14" stroke="#121212" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
          <!-- 小高亮背景方块 -->
          <rect x="14" y="9" width="4" height="4" rx="1" fill="#38BDF8" fill-opacity="0.4"/>
        </svg>
      </div>
    </div>

    <!-- 3. RAPL 实时功耗 & 电费 -->
    <div 
      class="toy-card p-3.5 bg-toy-yellow/30 flex items-center justify-between transition-transform hover:-translate-y-0.5"
      :style="{ boxShadow: '3.5px 3.5px 0px #121212' }"
    >
      <div>
        <div class="flex items-center gap-1.5">
          <span class="text-xs font-black text-toy-black uppercase tracking-wider">实时总功耗</span>
          <span class="text-[10px] font-bold px-1 bg-toy-yellow border border-toy-black rounded">
            {{ summary.measured_count }}台计量
          </span>
        </div>
        <div class="flex items-baseline gap-2 mt-1">
          <span class="text-2xl font-black font-mono text-toy-black">
            {{ summary.watts }} <span class="text-xs font-bold">W</span>
          </span>
          <span class="text-xs font-bold text-toy-muted">
            ¥{{ summary.est_cost_month }}/月
          </span>
        </div>
      </div>
      <!-- 纯 SVG 玩具能量闪电电池 (告别灯泡emoji) -->
      <div 
        class="w-10 h-10 rounded-xl bg-white border-2 border-toy-black flex items-center justify-center shrink-0 shadow-toy-sm"
        title="RAPL 实时功率"
      >
        <svg class="w-6 h-6" viewBox="0 0 24 24" fill="none">
          <!-- 电池外壳 -->
          <rect x="4" y="6" width="14" height="13" rx="2.5" fill="#FFE047" stroke="#121212" stroke-width="2.2"/>
          <path d="M9 3H13V6H9V3Z" fill="#121212"/>
          <!-- 闪电折线 -->
          <path d="M11 9L8 13H13L10 17" stroke="#121212" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </div>
    </div>

    <!-- 4. 今日全网流量 -->
    <div 
      class="toy-card p-3.5 bg-white flex items-center justify-between transition-transform hover:-translate-y-0.5"
      :style="{ boxShadow: '3.5px 3.5px 0px #121212' }"
    >
      <div>
        <span class="text-xs font-black text-toy-black uppercase tracking-wider block">今日消耗流量</span>
        <div class="flex items-baseline gap-2 mt-1">
          <span class="text-2xl font-black font-mono text-toy-black">
            {{ formatBytes(summary.day_in_total + summary.day_out_total) }}
          </span>
          <span class="text-xs font-mono text-toy-muted">
            (入:{{ formatBytes(summary.day_in_total) }})
          </span>
        </div>
      </div>
      <!-- 纯 SVG 玩具数据存储盒 (告别纸箱emoji) -->
      <div 
        class="w-10 h-10 rounded-xl bg-toy-bg border-2 border-toy-black flex items-center justify-center shrink-0 shadow-toy-sm"
        title="全网总流量积木"
      >
        <svg class="w-6 h-6" viewBox="0 0 24 24" fill="none">
          <!-- 磁盘盒/存储抽屉 -->
          <rect x="4" y="4" width="16" height="7" rx="1.5" fill="#FFFFFF" stroke="#121212" stroke-width="2.2"/>
          <line x1="7" y1="7.5" x2="11" y2="7.5" stroke="#121212" stroke-width="1.8" stroke-linecap="round"/>
          <circle cx="16" cy="7.5" r="1.2" fill="#00E599"/>

          <rect x="4" y="13" width="16" height="7" rx="1.5" fill="#FFFFFF" stroke="#121212" stroke-width="2.2"/>
          <line x1="7" y1="16.5" x2="11" y2="16.5" stroke="#121212" stroke-width="1.8" stroke-linecap="round"/>
          <circle cx="16" cy="16.5" r="1.2" fill="#FFE047"/>
        </svg>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { PublicSummary } from '../../types/backend';
import { formatBytes, formatBps } from '../../services/format';

defineProps<{
  summary: PublicSummary;
}>();
</script>
