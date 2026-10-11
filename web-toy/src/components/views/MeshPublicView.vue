<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏标题 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>WG 组网状态</span>
          <span class="toy-badge bg-toy-mint text-[10px]">Mesh 在线</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">WireGuard 全自动星型组网 · 主备中心巡检 · 流量审计</p>
      </div>

      <div class="toy-badge bg-toy-yellow text-xs">
        今日中转流量: {{ formatBytes(wg.trafficToday || 0) }}
      </div>
    </div>

    <!-- 1. 主备中心卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
      <!-- 现役主中心 -->
      <div class="toy-card p-4 bg-white border-2.5 border-toy-black flex items-start justify-between">
        <div class="space-y-1.5">
          <div class="flex items-center gap-2">
            <span class="text-base">👑</span>
            <span class="font-black text-sm text-toy-black">现役主中心 (Active Hub)</span>
            <span class="toy-badge bg-toy-mint text-[10px]">ACTIVE</span>
          </div>
          <div class="text-base font-extrabold text-toy-black">Tokyo-Master-01</div>
          <p class="text-xs font-mono text-toy-muted">
            本月中转: RX {{ formatBytes(wg.hubViews?.[0]?.month_rx || 0) }} / TX {{ formatBytes(wg.hubViews?.[0]?.month_tx || 0) }}
          </p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-toy-yellow border-2 border-toy-black flex items-center justify-center font-bold text-xs shrink-0 shadow-toy-sm">
          HUB
        </div>
      </div>

      <!-- 温备援中心 -->
      <div class="toy-card p-4 bg-white border-2.5 border-toy-black flex items-start justify-between">
        <div class="space-y-1.5">
          <div class="flex items-center gap-2">
            <span class="text-base">🛡️</span>
            <span class="font-black text-sm text-toy-black">温备援中心 (Standby Hub)</span>
            <span class="toy-badge bg-toy-gray text-[10px]">STANDBY</span>
          </div>
          <div class="text-base font-extrabold text-toy-black">HongKong-Edge-02</div>
          <p class="text-xs font-mono text-toy-muted">
            就绪状态: 配置实时对齐 · 支持无缝接管
          </p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-gray-100 border-2 border-toy-black flex items-center justify-center font-bold text-xs shrink-0 shadow-toy-sm">
          BAK
        </div>
      </div>
    </div>

    <!-- 2. 星型拓扑与成员列表 -->
    <div class="toy-card p-5 bg-white space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-black text-toy-black">在网成员节点列表 ({{ peerList.length }})</h3>
        <span class="text-xs font-mono text-toy-muted font-bold">每 5 分钟巡检一次</span>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
              <th class="pb-2">成员名称</th>
              <th class="pb-2">成员类型</th>
              <th class="pb-2">虚拟 IP</th>
              <th class="pb-2">最后握手</th>
              <th class="pb-2 text-right">互通状态</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-toy-gray font-mono">
            <tr v-for="p in peerList" :key="p.id" class="hover:bg-toy-bg/50 transition-colors">
              <td class="py-2.5 font-bold font-sans text-toy-black">{{ p.name }}</td>
              <td class="py-2.5">
                <span class="px-1.5 py-0.5 rounded text-[10px] font-bold border border-toy-black" :class="p.isHub ? 'bg-toy-yellow' : 'bg-toy-bg'">
                  {{ p.role }}
                </span>
              </td>
              <td class="py-2.5 text-toy-muted">{{ p.ip }}</td>
              <td class="py-2.5 text-toy-black font-bold">{{ p.handshake }}</td>
              <td class="py-2.5 text-right">
                <span class="toy-badge bg-toy-mint text-[10px]">● 在线</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { WGPublicSnapshot } from '../../types/backend';
import { formatBytes } from '../../services/mock';

defineProps<{
  wg: WGPublicSnapshot;
}>();

const peerList = [
  { id: 1, name: 'Tokyo-Master-01', role: '中心 Hub', isHub: true, ip: '10.66.66.1/24', handshake: '3秒前' },
  { id: 2, name: 'HongKong-Edge-02', role: '备援 Hub', isHub: true, ip: '10.66.66.2/24', handshake: '12秒前' },
  { id: 3, name: 'HomeLab-NAS-Intel', role: '内网成员', isHub: false, ip: '10.66.66.10/24', handshake: '28秒前' },
  { id: 4, name: 'Singapore-Cloud-ARM', role: '云端节点', isHub: false, ip: '10.66.66.11/24', handshake: '45秒前' },
  { id: 5, name: 'Frankfurt-Backup-DE', role: '灾备节点', isHub: false, ip: '10.66.66.12/24', handshake: '1分钟前' },
  { id: 6, name: 'MacBook-Pro-Work', role: '移动设备', isHub: false, ip: '10.66.66.50/24', handshake: '2分钟前' },
];
</script>
