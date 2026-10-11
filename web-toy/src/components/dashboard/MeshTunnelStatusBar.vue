<template>
  <div 
    class="toy-card p-3 bg-white flex items-center justify-between flex-wrap gap-3 select-none"
    :style="{ boxShadow: '3px 3px 0px #121212' }"
  >
    <!-- 左侧：WG 组网 -->
    <div class="flex items-center gap-3">
      <div class="flex items-center gap-2">
        <span class="w-2.5 h-2.5 rounded-full bg-toy-mint animate-pulse border border-toy-black"></span>
        <span class="text-xs font-black text-toy-black flex items-center gap-1.5">
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none">
            <path d="M12 3L4 6V12C4 17 8 20 12 21C16 20 20 17 20 12V6L12 3Z" fill="#E0F7FE" stroke="#121212" stroke-width="2"/>
            <path d="M9 12L11 14L15 10" stroke="#121212" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          <span>WireGuard Mesh 星网</span>
          <span class="px-1.5 py-0.2 text-[10px] bg-toy-mint text-toy-black border border-toy-black rounded">
            正常运转
          </span>
        </span>
      </div>
      <div class="hidden sm:flex items-center gap-2 text-xs font-mono text-toy-muted font-bold">
        <span>主中心: Tokyo-01</span>
        <span>·</span>
        <span>在线成员: {{ wg.onlineCount || 6 }} 台</span>
        <span>·</span>
        <span>今日中转: {{ formatBytes(wg.trafficToday || 0) }}</span>
      </div>
    </div>

    <!-- 右侧：内网穿透 -->
    <div class="flex items-center gap-3">
      <div class="flex items-center gap-1.5 text-xs font-black text-toy-black">
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none">
          <path d="M4 14H10L8 20L15 10H9L11 4L4 14Z" fill="#FFE047" stroke="#121212" stroke-width="2" stroke-linejoin="round"/>
        </svg>
        <span>穿透隧道:</span>
        <span class="font-mono text-toy-pink font-extrabold">{{ frp.sumOnline }}/{{ frp.sumTunnel }} 在线</span>
      </div>
      <div class="flex items-center gap-1">
        <span 
          v-for="p in frp.platforms" 
          :key="p.id"
          class="px-1.5 py-0.5 text-[10px] font-bold border border-toy-black rounded"
          :class="p.onlineTunnels > 0 ? 'bg-toy-yellow/40 text-toy-black' : 'bg-gray-100 text-toy-muted'"
          :title="`${p.name}: ${p.onlineTunnels}/${p.totalTunnels} 隧道在线`"
        >
          {{ p.name }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { WGPublicSnapshot, FRPPublicSnapshot } from '../../types/backend';
import { formatBytes } from '../../services/mock';

defineProps<{
  wg: WGPublicSnapshot;
  frp: FRPPublicSnapshot;
}>();
</script>
