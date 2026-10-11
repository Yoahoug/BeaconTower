<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏标题 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>穿透状态</span>
          <span class="toy-badge bg-toy-yellow text-[10px]">3 平台接入</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">Sakura FRP · ChmlFrp · Cloudflare Tunnel 统一用量与隧道健康度</p>
      </div>

      <div class="toy-badge bg-toy-mint text-xs">
        今日全网穿透流量: {{ formatBytes(frp.sumToday) }}
      </div>
    </div>

    <!-- 1. 三大平台指标卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
      <div 
        v-for="platform in frp.platforms" 
        :key="platform.id"
        class="toy-card p-4 bg-white flex flex-col justify-between space-y-3"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <div class="w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-yellow/30 flex items-center justify-center font-black text-xs">
              {{ platform.type.slice(0, 2).toUpperCase() }}
            </div>
            <div>
              <h4 class="font-black text-sm text-toy-black">{{ platform.name }}</h4>
              <span class="text-[10px] font-bold text-toy-muted uppercase">{{ platform.type }}</span>
            </div>
          </div>
          <span class="toy-badge bg-toy-mint text-[10px]">
            {{ platform.onlineTunnels }}/{{ platform.totalTunnels }} 活跃
          </span>
        </div>

        <div class="p-2 bg-toy-bg rounded-lg border border-toy-black space-y-1 text-xs font-mono">
          <div class="flex justify-between text-toy-muted">
            <span>今日消耗流量:</span>
            <span class="font-extrabold text-toy-black">{{ formatBytes(platform.todayTraffic) }}</span>
          </div>
          <div class="flex justify-between text-toy-muted">
            <span>配额状态:</span>
            <span class="font-extrabold text-toy-mint">● 额度充足</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 2. 统一穿透隧道列表 -->
    <div class="toy-card p-5 bg-white space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-black text-toy-black">全部活跃穿透隧道 ({{ tunnelList.length }})</h3>
        <span class="text-xs font-mono text-toy-muted font-bold">每 30 秒自动同步</span>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
              <th class="pb-2">隧道名称</th>
              <th class="pb-2">所属平台</th>
              <th class="pb-2">协议与目标</th>
              <th class="pb-2">映射节点</th>
              <th class="pb-2">今日流量</th>
              <th class="pb-2 text-right">在线状态</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-toy-gray font-mono">
            <tr v-for="t in tunnelList" :key="t.id" class="hover:bg-toy-bg/50 transition-colors">
              <td class="py-2.5 font-bold font-sans text-toy-black">{{ t.name }}</td>
              <td class="py-2.5">
                <span class="px-1.5 py-0.5 rounded bg-toy-gray text-[10px] font-bold border border-toy-black">
                  {{ t.platform }}
                </span>
              </td>
              <td class="py-2.5 text-toy-muted">{{ t.protocol }}://{{ t.target }}</td>
              <td class="py-2.5 font-bold text-toy-black">{{ t.node }}</td>
              <td class="py-2.5 text-sky-800 font-bold">{{ t.todayTraffic }}</td>
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
import { FRPPublicSnapshot } from '../../types/backend';
import { formatBytes } from '../../services/mock';

defineProps<{
  frp: FRPPublicSnapshot;
}>();

const tunnelList = [
  { id: 1, name: 'sakura-ssh-jap', platform: 'Sakura FRP', protocol: 'tcp', target: '127.0.0.1:22', node: 'Tokyo-Master-01', todayTraffic: '14.2 GB' },
  { id: 2, name: 'chml-ingress-hk', platform: 'ChmlFrp', protocol: 'http', target: 'demo.hk.ingress:80', node: 'HongKong-Edge-02', todayTraffic: '6.8 GB' },
  { id: 3, name: 'cf-tunnel-managed', platform: 'Cloudflare', protocol: 'https', target: 'beacontower.managed', node: 'Tokyo-Master-01', todayTraffic: '3.8 GB' },
  { id: 4, name: 'sakura-nas-drive', platform: 'Sakura FRP', protocol: 'tcp', target: '127.0.0.1:5000', node: 'HomeLab-NAS-Intel', todayTraffic: '2.4 GB' },
];
</script>
