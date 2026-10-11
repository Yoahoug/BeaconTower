<template>
  <div 
    class="toy-card p-4 transition-all duration-200 relative overflow-hidden"
    :class="[
      server.status === 'offline' ? 'opacity-70 bg-gray-50/90' : 'hover:-translate-y-0.5 hover:shadow-toy-lg',
      isExpanded ? 'ring-2 ring-toy-black' : ''
    ]"
  >
    <!-- 卡片顶栏：头像 + 名称 + 标签 + 在线状态 -->
    <div class="flex items-start justify-between gap-3">
      <div class="flex items-center gap-3 min-w-0">
        <!-- 头像插槽：根据全局设置切换为 [像素机器人] 或 [系统发行版徽标] -->
        <div class="shrink-0 cursor-pointer" @click="$emit('toggle-avatar')" title="点击切换头像风格">
          <PixelBotAvatar 
            v-if="avatarMode === 'bot'"
            :seed="server.name + server.id" 
            :is-online="server.status === 'online'" 
            size="md"
          />
          <OSDistroBadge 
            v-else
            :os-name="server.profile.os" 
            :is-online="server.status === 'online'" 
            size="md"
          />
        </div>

        <!-- 服务器名称与标签 -->
        <div class="min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <h3 class="font-extrabold text-base text-toy-black truncate leading-tight tracking-tight">
              {{ server.name }}
            </h3>
            <!-- 地区邮票贴纸 -->
            <span class="inline-flex items-center gap-1 px-1.5 py-0.5 text-[11px] font-bold bg-toy-yellow/30 border border-toy-black rounded text-toy-black">
              {{ formatRegionFlag(server.region) }} {{ server.region }}
            </span>
            <!-- 自定义标签 -->
            <span 
              v-for="tag in server.tags" 
              :key="tag"
              class="px-1.5 py-0.5 text-[10px] font-extrabold bg-toy-gray border border-toy-black rounded text-toy-black"
            >
              {{ tag }}
            </span>
          </div>

          <!-- 系统与硬件规格副标题 -->
          <div class="text-[11px] font-bold text-toy-muted flex items-center gap-1.5 mt-0.5 truncate">
            <span>{{ server.profile.os }} {{ server.profile.os_version }}</span>
            <span>·</span>
            <span>{{ server.profile.cpu_cores }} Cores</span>
            <span>·</span>
            <span>{{ formatBytes(server.profile.mem_total) }}</span>
            <span v-if="server.profile.virt && server.profile.virt !== 'none'" class="uppercase text-[9px] bg-toy-black/5 px-1 rounded">
              {{ server.profile.virt }}
            </span>
          </div>
        </div>
      </div>

      <!-- 在线/离线状态胶囊 -->
      <div class="shrink-0">
        <span 
          v-if="server.status === 'online'"
          class="toy-badge bg-toy-mint text-toy-black"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-toy-black animate-pulse"></span>
          ONLINE
        </span>
        <span 
          v-else
          class="toy-badge bg-gray-200 text-toy-muted border-gray-400 shadow-none"
        >
          OFFLINE
        </span>
      </div>
    </div>

    <!-- 核心指标条：CPU、内存、磁盘 (高对比度、大字清晰) -->
    <div class="mt-4 space-y-2.5">
      <!-- 1. CPU -->
      <div>
        <div class="flex items-center justify-between text-xs font-black text-toy-black mb-1">
          <span class="tracking-wide">CPU 使用率</span>
          <span class="font-mono text-sm" :class="server.metrics.cpu_pct > 80 ? 'text-toy-pink' : 'text-toy-black'">
            {{ server.status === 'online' ? server.metrics.cpu_pct.toFixed(1) + '%' : '--' }}
          </span>
        </div>
        <div class="toy-groove h-3.5 flex items-center p-0.5 bg-gray-100">
          <div 
            class="h-full rounded transition-all duration-300 border border-toy-black"
            :class="server.metrics.cpu_pct > 80 ? 'bg-toy-pink' : 'bg-toy-yellow'"
            :style="{ width: server.status === 'online' ? `${server.metrics.cpu_pct}%` : '0%' }"
          ></div>
        </div>
      </div>

      <!-- 2. RAM -->
      <div>
        <div class="flex items-center justify-between text-xs font-black text-toy-black mb-1">
          <span class="tracking-wide">内存 RAM</span>
          <span class="font-mono text-xs">
            <template v-if="server.status === 'online'">
              {{ formatBytes(server.metrics.mem_used) }} / {{ formatBytes(server.profile.mem_total) }}
              <span class="text-toy-muted font-bold ml-1">({{ memPercentage }}%)</span>
            </template>
            <template v-else>--</template>
          </span>
        </div>
        <div class="toy-groove h-3.5 flex items-center p-0.5 bg-gray-100">
          <div 
            class="h-full rounded transition-all duration-300 border border-toy-black bg-toy-cyan"
            :style="{ width: server.status === 'online' ? `${memPercentage}%` : '0%' }"
          ></div>
        </div>
      </div>

      <!-- 3. DISK -->
      <div>
        <div class="flex items-center justify-between text-xs font-black text-toy-black mb-1">
          <span class="tracking-wide">存储 磁盘</span>
          <span class="font-mono text-xs">
            <template v-if="server.status === 'online'">
              {{ formatBytes(server.metrics.disk_used) }} / {{ formatBytes(server.profile.disk_total) }}
              <span class="text-toy-muted font-bold ml-1">({{ diskPercentage }}%)</span>
            </template>
            <template v-else>--</template>
          </span>
        </div>
        <div class="toy-groove h-3.5 flex items-center p-0.5 bg-gray-100">
          <div 
            class="h-full rounded transition-all duration-300 border border-toy-black bg-toy-orange"
            :style="{ width: server.status === 'online' ? `${diskPercentage}%` : '0%' }"
          ></div>
        </div>
      </div>
    </div>

    <!-- 底部状态条：网速、功耗、温度、运行时长 -->
    <div class="mt-4 pt-3 border-t-2 border-toy-gray/80 flex items-center justify-between flex-wrap gap-2 text-xs font-extrabold text-toy-black">
      <!-- 实时双向速率 -->
      <div class="flex items-center gap-2 font-mono">
        <span class="inline-flex items-center gap-0.5 text-sky-700">
          <span>↑</span> {{ formatBps(server.metrics.net_out_bps) }}
        </span>
        <span class="inline-flex items-center gap-0.5 text-blue-700">
          <span>↓</span> {{ formatBps(server.metrics.net_in_bps) }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <!-- RAPL 功耗指示器（若支持） -->
        <span 
          v-if="server.power"
          class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-toy-yellow text-[11px] font-black border border-toy-black rounded"
          title="RAPL 实时功耗"
        >
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none">
            <path d="M13 2L3 14H12L11 22L21 10H12L13 2Z" fill="#121212" stroke="#121212" stroke-width="1.5" stroke-linejoin="round"/>
          </svg>
          <span>{{ server.power.total_w }}W</span>
        </span>

        <!-- 温度 -->
        <span 
          v-if="server.power?.temp_c" 
          class="inline-flex items-center gap-0.5 text-[11px] font-mono text-toy-muted"
          title="CPU 实时温度"
        >
          <svg class="w-3 h-3 text-toy-black" viewBox="0 0 24 24" fill="none">
            <path d="M14 14.76V3.5C14 2.67 13.33 2 12.5 2C11.67 2 11 2.67 11 3.5V14.76C9.8 15.42 9 16.71 9 18.2C9 20.3 10.7 22 12.8 22C14.9 22 16.6 20.3 16.6 18.2C16.6 16.71 15.8 15.42 14.6 14.76H14Z" stroke="#121212" stroke-width="2" fill="#FF5E85"/>
          </svg>
          <span>{{ server.power.temp_c }}°C</span>
        </span>

        <!-- 在线运行天数 -->
        <span class="inline-flex items-center gap-0.5 text-[11px] font-mono text-toy-muted" title="在线时长">
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none">
            <circle cx="12" cy="12" r="9" stroke="#121212" stroke-width="2"/>
            <polyline points="12 7 12 12 15 15" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
          </svg>
          <span>{{ formatUptime(server.metrics.uptime_s) }}</span>
        </span>
      </div>
    </div>

    <!-- 展开详情条 -->
    <div class="mt-2.5 flex items-center justify-between pt-2 border-t border-dashed border-toy-gray text-[11px]">
      <span class="text-toy-muted truncate max-w-[240px]" :title="server.note_public">
        {{ server.note_public || '无公开备注' }}
      </span>
      <div class="flex items-center gap-1.5">
        <button 
          @click="$emit('select-detail', server)"
          class="px-2 py-0.5 rounded text-[10px] font-extrabold border border-toy-black bg-toy-yellow hover:bg-yellow-300 transition-colors shadow-toy-sm cursor-pointer"
          title="查看该节点历史趋势曲线"
        >
          详情曲线 ➔
        </button>
        <button 
          @click="isExpanded = !isExpanded"
          class="px-2 py-0.5 rounded text-[10px] font-extrabold border border-toy-black bg-white hover:bg-toy-cyan transition-colors shadow-toy-sm cursor-pointer"
        >
          {{ isExpanded ? '收起 ▴' : '指标 ▾' }}
        </button>
      </div>
    </div>

    <!-- 展开后的深度指标明细 -->
    <div v-if="isExpanded" class="mt-3 p-3 rounded-lg bg-toy-bg border-2 border-toy-black space-y-2 text-xs">
      <div class="font-black text-toy-black border-b border-toy-black/20 pb-1 flex items-center gap-1.5">
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
          <rect x="3" y="3" width="18" height="18" rx="2" stroke="#121212" stroke-width="2"/>
          <line x1="3" y1="9" x2="21" y2="9" stroke="#121212" stroke-width="2"/>
          <line x1="9" y1="21" x2="9" y2="9" stroke="#121212" stroke-width="2"/>
        </svg>
        <span>详细运行指标与流量统计</span>
      </div>
      <div class="grid grid-cols-2 gap-2 font-mono text-[11px]">
        <div>负载均值: {{ server.metrics.load1 }} / {{ server.metrics.load5 }} / {{ server.metrics.load15 }}</div>
        <div>活动进程: {{ server.metrics.processes }} 个</div>
        <div>TCP 连接: {{ server.metrics.tcp_conns }} 条</div>
        <div>UDP 连接: {{ server.metrics.udp_conns }} 条</div>
        <div v-if="server.power">月度用电: {{ server.power.month_kwh }} kWh</div>
        <div v-if="server.power">预估电费: ¥{{ server.power.est_cost_month }}</div>
        <div v-if="server.traffic_today">今日下行: {{ formatBytes(server.traffic_today.in_total) }}</div>
        <div v-if="server.traffic_today">今日上行: {{ formatBytes(server.traffic_today.out_total) }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { ServerItem } from '../../types/backend';
import { formatBytes, formatBps, formatUptime } from '../../services/format';
import PixelBotAvatar from '../icons/PixelBotAvatar.vue';
import OSDistroBadge from '../icons/OSDistroBadge.vue';

const props = defineProps<{
  server: ServerItem;
  avatarMode: 'bot' | 'os';
}>();

defineEmits<{
  (e: 'toggle-status', id: number): void;
  (e: 'toggle-avatar'): void;
  (e: 'select-detail', server: ServerItem): void;
}>();

const isExpanded = ref(false);

const memPercentage = computed(() => {
  if (!props.server.profile.mem_total) return 0;
  return Math.round((props.server.metrics.mem_used / props.server.profile.mem_total) * 100);
});

const diskPercentage = computed(() => {
  if (!props.server.profile.disk_total) return 0;
  return Math.round((props.server.metrics.disk_used / props.server.profile.disk_total) * 100);
});

function formatRegionFlag(region: string): string {
  if (!region) return '🌐';
  if (region.includes('JP') || region.includes('日本') || region.includes('东京')) return '🇯🇵';
  if (region.includes('HK') || region.includes('香港')) return '🇭🇰';
  if (region.includes('CN') || region.includes('上海') || region.includes('北京') || region.includes('广州')) return '🇨🇳';
  if (region.includes('SG') || region.includes('新加坡')) return '🇸🇬';
  if (region.includes('DE') || region.includes('法兰克福') || region.includes('德国')) return '🇩🇪';
  if (region.includes('US') || region.includes('硅谷') || region.includes('美国')) return '🇺🇸';
  return '🌐';
}
</script>
