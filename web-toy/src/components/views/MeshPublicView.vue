<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏标题 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>WG 组网状态</span>
          <span class="toy-badge" :class="wg.initialized ? 'bg-toy-mint text-[10px]' : 'bg-gray-200 text-[10px] text-toy-muted'">
            {{ wg.initialized ? 'Mesh 运行中' : '未初始化' }}
          </span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">WireGuard 全自动星型组网 · 主备中心巡检 · 流量审计</p>
      </div>

      <div class="toy-badge bg-toy-yellow text-xs">
        今日中转流量: {{ formatBytes(wg.summary?.traffic_today || 0) }}
      </div>
    </div>

    <!-- 未初始化引导态（真实后端 initialized=false 时不伪造运行态） -->
    <div v-if="!wg.initialized" class="toy-card p-10 text-center bg-white space-y-2">
      <span class="text-4xl">🛰️</span>
      <h3 class="text-base font-black text-toy-black">组网尚未初始化</h3>
      <p class="text-xs font-bold text-toy-muted">管理员登录后可在「WG 组网管理」完成一键星型组网</p>
    </div>

    <template v-else>
      <!-- 1. 中心节点卡片（真实 hubs 视图） -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div
          v-for="hub in wg.hubs"
          :key="hub.name"
          class="toy-card p-4 bg-white border-2.5 border-toy-black flex items-start justify-between"
        >
          <div class="space-y-1.5">
            <div class="flex items-center gap-2">
              <span class="text-base">{{ hub.is_active ? '👑' : '🛡️' }}</span>
              <span class="font-black text-sm text-toy-black">{{ hub.is_active ? '现役主中心 (Active Hub)' : '温备援中心 (Standby)' }}</span>
              <span class="toy-badge text-[10px]" :class="hub.healthy ? 'bg-toy-mint text-toy-black' : 'bg-red-100 text-red-700'">
                {{ hub.healthy ? '健康' : '巡检异常' }}
              </span>
            </div>
            <div class="text-base font-extrabold text-toy-black">{{ hub.name || '未知节点' }}</div>
            <p class="text-xs font-mono text-toy-muted">
              本月中转: RX {{ formatBytes(hub.month_rx) }} / TX {{ formatBytes(hub.month_tx) }}
            </p>
          </div>
          <div class="w-10 h-10 rounded-xl border-2 border-toy-black flex items-center justify-center font-bold text-xs shrink-0 shadow-toy-sm"
               :class="hub.is_active ? 'bg-toy-yellow' : 'bg-gray-100'">
            {{ hub.is_active ? 'HUB' : 'BAK' }}
          </div>
        </div>
      </div>

      <!-- 2. 成员列表（真实 peers 视图：访客只看在线状态，不下发 IP/握手/流量） -->
      <div class="toy-card p-5 bg-white space-y-4">
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-black text-toy-black">在网成员节点列表 ({{ wg.peers.length }})</h3>
          <span class="text-xs font-mono text-toy-muted font-bold">
            成员在线 {{ wg.summary?.member_online ?? 0 }}/{{ wg.summary?.member_total ?? 0 }} · 外部终端 {{ wg.summary?.device_total ?? 0 }} 台
          </span>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead>
              <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                <th class="pb-2">成员名称</th>
                <th class="pb-2 text-right">互通状态</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-toy-gray font-mono">
              <tr v-for="p in wg.peers" :key="p.name" class="hover:bg-toy-bg/50 transition-colors">
                <td class="py-2.5 font-bold font-sans text-toy-black">{{ p.name }}</td>
                <td class="py-2.5 text-right">
                  <span class="toy-badge text-[10px]" :class="p.online ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'">
                    {{ p.online ? '● 在线' : '○ 离线' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import type { WGPublicSnapshot } from '../../types/backend';
import { formatBytes } from '../../services/format';

defineProps<{
  wg: WGPublicSnapshot;
}>();
</script>
