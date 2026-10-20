<template>
  <div class="toy-card px-4 py-3 bg-white flex items-center justify-between flex-wrap gap-x-6 gap-y-2 select-none">
    <!-- 左侧：WG 组网 -->
    <div class="flex items-center gap-2.5 min-w-0">
      <span
        class="w-2 h-2 rounded-full shrink-0 border border-toy-black/50"
        :class="wgReady ? 'bg-toy-mint' : 'bg-toy-gray'"
      ></span>
      <span class="inline-flex items-center gap-1.5 text-xs font-black text-toy-black whitespace-nowrap">
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none">
          <path d="M12 3L4 6V12C4 17 8 20 12 21C16 20 20 17 20 12V6L12 3Z" fill="#E0F7FE" stroke="#121212" stroke-width="2"/>
          <path d="M9 12L11 14L15 10" stroke="#121212" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        WireGuard 组网
        <span
          class="px-1.5 py-0.5 text-[9px] font-black border border-toy-black rounded"
          :class="wgReady ? 'bg-toy-mint/30 text-emerald-800' : 'bg-toy-gray/50 text-toy-muted'"
        >
          {{ wgReady ? '运行中' : '未初始化' }}
        </span>
      </span>
      <span class="hidden md:inline text-[10px] font-bold text-toy-muted truncate">
        {{ activeHubName ? `中心 ${activeHubName}` : '未配置中心' }}
        · 成员 {{ wg.summary?.member_online ?? 0 }}/{{ wg.summary?.member_total ?? 0 }}
        · 今日中转 {{ formatBytes(wg.summary?.traffic_today || 0) }}
      </span>
    </div>

    <!-- 右侧：内网穿透 -->
    <div class="flex items-center gap-2.5 min-w-0">
      <span class="inline-flex items-center gap-1.5 text-xs font-black text-toy-black whitespace-nowrap">
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none">
          <path d="M4 14H10L8 20L15 10H9L11 4L4 14Z" fill="#FFE047" stroke="#121212" stroke-width="2" stroke-linejoin="round"/>
        </svg>
        穿透隧道
        <span class="font-mono" :class="frpOnline ? 'text-emerald-700' : 'text-toy-pink'">
          {{ frp.summary?.tunnel_online ?? 0 }}/{{ frp.summary?.tunnel_total ?? 0 }}
        </span>
      </span>
      <div class="flex items-center gap-1">
        <span
          v-for="p in frp.platforms"
          :key="p.name"
          class="px-1.5 py-0.5 text-[9px] font-bold border border-toy-black/60 rounded"
          :class="p.online ? 'bg-toy-bg text-toy-black' : 'bg-gray-100 text-toy-muted'"
          :title="`${p.name}: ${p.tunnel_online}/${p.tunnel_total} 隧道在线`"
        >
          {{ p.name }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { WGPublicSnapshot, FRPPublicSnapshot } from '../../types/backend';
import { formatBytes } from '../../services/format';

const props = defineProps<{
  wg: WGPublicSnapshot;
  frp: FRPPublicSnapshot;
}>();

const activeHubName = computed(() => props.wg.hubs?.find((h) => h.is_active)?.name || '');
const wgReady = computed(() => !!props.wg.initialized && (props.wg.summary?.hub_healthy ?? 0) > 0);
const frpOnline = computed(() => (props.frp.summary?.tunnel_online ?? 0) > 0);
</script>
