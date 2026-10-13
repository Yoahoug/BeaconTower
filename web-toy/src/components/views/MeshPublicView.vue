<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏标题 -->
    <div class="flex items-center justify-between flex-wrap gap-3 mb-2">
      <div class="space-y-1.5">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h2 class="text-xl font-black text-toy-black">WG 组网状态</h2>
          <span class="toy-badge text-[11px] px-2.5 py-0.5" :class="wg.initialized ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-zinc-500'">
            {{ wg.initialized ? 'Mesh 运行中' : '未初始化' }}
          </span>
        </div>
        <p class="text-xs font-bold text-zinc-500">WireGuard 全自动星型组网 · 主备中心巡检 · 流量审计</p>
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
      <!-- 0. 组网关键指标（对齐 classic 组网页 KPI 条口径） -->
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <div
          class="toy-card p-3.5"
          :class="hubAbnormal ? 'bg-toy-pink/25' : 'bg-toy-cyan/20'"
          :style="{ boxShadow: '3.5px 3.5px 0px #121212' }"
        >
          <span class="text-[11px] font-black text-toy-black uppercase tracking-wider block">中心节点</span>
          <div class="text-xl font-black font-mono text-toy-black mt-1">
            {{ wg.summary?.hub_healthy ?? 0 }} <span class="text-xs font-bold text-toy-muted">/ {{ wg.summary?.hub_active ?? 0 }}</span>
          </div>
          <span class="text-[10px] font-bold text-toy-muted">
            {{ hubAbnormal ? '存在巡检异常的中心' : '现役与备援均健康' }}
          </span>
        </div>
        <div
          class="toy-card p-3.5"
          :class="memberOffline ? 'bg-toy-pink/25' : 'bg-toy-mint/25'"
          :style="{ boxShadow: '3.5px 3.5px 0px #121212' }"
        >
          <span class="text-[11px] font-black text-toy-black uppercase tracking-wider block">成员在线</span>
          <div class="text-xl font-black font-mono text-toy-black mt-1">
            {{ wg.summary?.member_online ?? 0 }} <span class="text-xs font-bold text-toy-muted">/ {{ wg.summary?.member_total ?? 0 }}</span>
          </div>
          <span class="text-[10px] font-bold text-toy-muted">{{ memberOffline ? `${memberOffline} 台离线` : '全部在线' }}</span>
        </div>
        <div class="toy-card p-3.5 bg-toy-yellow/30" :style="{ boxShadow: '3.5px 3.5px 0px #121212' }">
          <span class="text-[11px] font-black text-toy-black uppercase tracking-wider block">在网设备</span>
          <div class="text-xl font-black font-mono text-toy-black mt-1">{{ wg.summary?.device_total ?? 0 }}</div>
          <span class="text-[10px] font-bold text-toy-muted">手机 / 电脑等使用端</span>
        </div>
        <div class="toy-card p-3.5 bg-white" :style="{ boxShadow: '3.5px 3.5px 0px #121212' }">
          <span class="text-[11px] font-black text-toy-black uppercase tracking-wider block">今日中转流量</span>
          <div class="text-xl font-black font-mono text-toy-black mt-1">{{ formatBytes(wg.summary?.traffic_today || 0) }}</div>
          <span class="text-[10px] font-bold text-toy-muted">本月累计 {{ formatBytes(wg.summary?.traffic_month || 0) }}</span>
        </div>
      </div>

      <!-- 1. 中心节点卡片（真实 hubs 视图） -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div
          v-for="hub in wg.hubs"
          :key="hub.name"
          class="toy-card p-4 bg-white border-2.5 border-toy-black flex items-start justify-between"
        >
          <div class="space-y-1.5">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-base">{{ hub.is_active ? '👑' : '🛡️' }}</span>
              <span class="font-black text-sm text-toy-black">{{ hub.is_active ? '现役主中心 (Active Hub)' : '温备援中心 (Standby)' }}</span>
              <span class="toy-badge bg-toy-cyan/40 text-[10px]">{{ hub.is_active ? '现役' : '备援' }}</span>
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
        <div class="flex items-center justify-between flex-wrap gap-2">
          <h3 class="text-sm font-black text-toy-black">在网成员节点列表 ({{ wg.peers.length }})</h3>
          <span class="text-xs font-mono text-toy-muted font-bold">
            成员在线 {{ wg.summary?.member_online ?? 0 }}/{{ wg.summary?.member_total ?? 0 }} · 外部终端 {{ wg.summary?.device_total ?? 0 }} 台 · {{ checkedText }}
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
              <tr
                v-for="p in wg.peers"
                :key="p.name"
                class="transition-colors"
                :class="p.online ? 'hover:bg-toy-bg/50' : 'opacity-60'"
              >
                <td class="py-2.5 font-bold font-sans text-toy-black">{{ p.name }}</td>
                <td class="py-2.5 text-right">
                  <span class="toy-badge text-[10px]" :class="p.online ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'">
                    {{ p.online ? '● 在线' : '○ 离线' }}
                  </span>
                </td>
              </tr>
              <tr v-if="!wg.peers.length">
                <td colspan="2" class="py-5 text-center text-toy-muted font-bold font-sans">暂无网内服务器成员</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { WGPublicSnapshot } from '../../types/backend';
import { formatBytes } from '../../services/format';

const props = defineProps<{
  wg: WGPublicSnapshot;
}>();

const hubAbnormal = computed(() =>
  Math.max(0, (props.wg.summary?.hub_active || 0) - (props.wg.summary?.hub_healthy || 0)),
);
const memberOffline = computed(() =>
  Math.max(0, (props.wg.summary?.member_total || 0) - (props.wg.summary?.member_online || 0)),
);

// 中心「上次巡检」是巡检 ticker 回写的 unix 秒（5 分钟一轮）
const checkedText = computed(() => {
  const ts = Number(props.wg.summary?.checked_at) || 0;
  if (!ts) return '尚未巡检';
  const ago = Math.max(0, Math.floor(Date.now() / 1000) - ts);
  if (ago < 60) return `巡检于 ${ago} 秒前`;
  if (ago < 3600) return `巡检于 ${Math.floor(ago / 60)} 分钟前`;
  if (ago < 86400) return `巡检于 ${Math.floor(ago / 3600)} 小时前`;
  return `巡检于 ${Math.floor(ago / 86400)} 天前`;
});
</script>
