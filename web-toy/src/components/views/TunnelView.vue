<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏标题 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>穿透状态</span>
          <span class="toy-badge bg-toy-yellow text-[10px]">{{ frp.summary?.platform_total || 0 }} 平台接入</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">Sakura FRP · ChmlFrp · Cloudflare Tunnel 统一用量与隧道健康度</p>
      </div>

      <div class="toy-badge bg-toy-mint text-xs">
        今日全网穿透流量: {{ formatBytes(frp.summary?.traffic_today || 0) }}
      </div>
    </div>

    <!-- 空态（真实后端未绑定任何平台） -->
    <div v-if="!frp.platforms.length" class="toy-card p-10 text-center bg-white space-y-2">
      <span class="text-4xl">🔌</span>
      <h3 class="text-base font-black text-toy-black">暂未接入穿透平台</h3>
      <p class="text-xs font-bold text-toy-muted">管理员登录后可在「内网穿透托管」绑定 Sakura / ChmlFrp / Cloudflare</p>
    </div>

    <!-- 1. 平台指标卡片（真实 platforms 视图） -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
      <div
        v-for="platform in frp.platforms"
        :key="platform.name"
        class="toy-card p-4 bg-white flex flex-col justify-between space-y-3"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <div class="w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-yellow/30 flex items-center justify-center font-black text-xs">
              {{ platform.kind.slice(0, 2).toUpperCase() }}
            </div>
            <div>
              <h4 class="font-black text-sm text-toy-black">{{ platform.name }}</h4>
              <span class="text-[10px] font-bold text-toy-muted uppercase">{{ platform.kind }}</span>
            </div>
          </div>
          <span class="toy-badge text-[10px]" :class="platform.online ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'">
            {{ platform.online ? '正常' : '异常' }}
          </span>
        </div>

        <div class="p-2 bg-toy-bg rounded-lg border border-toy-black space-y-1 text-xs font-mono">
          <div class="flex justify-between text-toy-muted">
            <span>隧道:</span>
            <span class="font-extrabold text-toy-black">{{ platform.tunnel_online }}/{{ platform.tunnel_total }} 活跃</span>
          </div>
          <div class="flex justify-between text-toy-muted">
            <span>今日消耗流量:</span>
            <span class="font-extrabold text-toy-black">{{ formatBytes(platform.traffic_today) }}</span>
          </div>
          <div class="flex justify-between text-toy-muted">
            <span>在用节点:</span>
            <span class="font-extrabold text-toy-black">
              {{ (platform.nodes || []).filter((n) => n.online).length }}/{{ (platform.nodes || []).length }} 在线
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- 2. 全网汇总条 -->
    <div class="toy-card p-3.5 bg-toy-bg border-2 border-toy-black flex items-center justify-between flex-wrap gap-2 text-xs font-bold font-mono">
      <span>隧道总数: <span class="text-toy-black font-extrabold">{{ frp.summary?.tunnel_total ?? 0 }}</span> · 在线 <span class="text-toy-black font-extrabold">{{ frp.summary?.tunnel_online ?? 0 }}</span></span>
      <span>当前连接: <span class="text-toy-black font-extrabold">{{ frp.summary?.conns ?? 0 }}</span></span>
      <span class="hidden sm:inline">在用节点: {{ frp.summary?.node_online ?? 0 }}/{{ frp.summary?.node_in_use ?? 0 }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { FRPPublicSnapshot } from '../../types/backend';
import { formatBytes } from '../../services/format';

defineProps<{
  frp: FRPPublicSnapshot;
}>();
</script>
