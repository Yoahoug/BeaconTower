<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏标题 -->
    <div class="flex items-center justify-between flex-wrap gap-3 mb-2">
      <div class="space-y-1.5">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h2 class="text-xl font-black text-toy-black">穿透状态</h2>
          <span class="toy-badge bg-toy-yellow text-[11px] px-2.5 py-0.5">{{ frp.summary?.platform_total || 0 }} 平台接入</span>
        </div>
        <p class="text-xs font-bold text-zinc-500">Sakura FRP · ChmlFrp · Cloudflare Tunnel 统一用量与隧道健康度 · 每 60 秒刷新</p>
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

    <template v-else>
      <!-- 0. 全网关键指标（对齐 classic 穿透页 KPI 条口径） -->
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <div class="toy-card p-3.5 bg-toy-mint/25" :style="{ boxShadow: '3.5px 3.5px 0px #121212' }">
          <span class="text-[11px] font-black text-toy-black uppercase tracking-wider block">今日流量消耗</span>
          <div class="text-xl font-black font-mono text-toy-black mt-1">{{ formatBytes(frp.summary?.traffic_today || 0) }}</div>
          <span class="text-[10px] font-bold text-toy-muted">{{ frp.summary?.platform_total || 0 }} 个平台合计</span>
        </div>
        <div
          class="toy-card p-3.5"
          :class="offlineTunnels ? 'bg-toy-pink/25' : 'bg-toy-cyan/20'"
          :style="{ boxShadow: '3.5px 3.5px 0px #121212' }"
        >
          <span class="text-[11px] font-black text-toy-black uppercase tracking-wider block">在线隧道</span>
          <div class="text-xl font-black font-mono text-toy-black mt-1">
            {{ frp.summary?.tunnel_online ?? 0 }} <span class="text-xs font-bold text-toy-muted">/ {{ frp.summary?.tunnel_total ?? 0 }}</span>
          </div>
          <span class="text-[10px] font-bold text-toy-muted">{{ offlineTunnels ? `${offlineTunnels} 条离线` : '全部在线' }}</span>
        </div>
        <div
          class="toy-card p-3.5"
          :class="nodesOffline ? 'bg-toy-pink/25' : 'bg-toy-yellow/30'"
          :style="{ boxShadow: '3.5px 3.5px 0px #121212' }"
        >
          <span class="text-[11px] font-black text-toy-black uppercase tracking-wider block">在用节点</span>
          <div class="text-xl font-black font-mono text-toy-black mt-1">
            {{ frp.summary?.node_online ?? 0 }} <span class="text-xs font-bold text-toy-muted">/ {{ frp.summary?.node_in_use ?? 0 }}</span>
          </div>
          <span class="text-[10px] font-bold text-toy-muted">{{ nodesOffline ? `${nodesOffline} 个节点离线` : '隧道所落节点全部在线' }}</span>
        </div>
        <div class="toy-card p-3.5 bg-white" :style="{ boxShadow: '3.5px 3.5px 0px #121212' }">
          <span class="text-[11px] font-black text-toy-black uppercase tracking-wider block">实时连接数</span>
          <div class="text-xl font-black font-mono text-toy-black mt-1">
            <template v-if="connsSupported">{{ frp.summary?.conns ?? 0 }}</template>
            <template v-else>—</template>
          </div>
          <span class="text-[10px] font-bold text-toy-muted">
            {{ connsSupported ? '活跃连接（面板节点实测）' : '平台未提供 · 面板不可测' }}
          </span>
        </div>
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
              <div class="w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-yellow/30 flex items-center justify-center font-black text-[10px]">
                {{ kindLabel(platform.kind).slice(0, 2).toUpperCase() }}
              </div>
              <div>
                <h4 class="font-black text-sm text-toy-black">{{ platform.name }}</h4>
                <span class="text-[10px] font-bold text-toy-muted">{{ kindLabel(platform.kind) }}</span>
              </div>
            </div>
            <span class="toy-badge text-[10px]" :class="platform.online ? 'bg-toy-mint text-toy-black' : 'bg-toy-orange/70 text-toy-black'">
              {{ platform.online ? '同步正常' : '同步异常' }}
            </span>
          </div>

          <div class="p-2 bg-toy-bg rounded-lg border border-toy-black space-y-1 text-xs font-mono">
            <div class="flex justify-between text-toy-muted">
              <span>今日流量:</span>
              <!-- Cloudflare 是边缘转发模型，平台不给当日流量口径 -->
              <span v-if="platform.kind === 'cloudflared'" class="font-extrabold text-toy-muted">—</span>
              <span v-else class="font-extrabold text-toy-black">{{ formatBytes(platform.traffic_today) }}</span>
            </div>
            <div class="flex justify-between text-toy-muted">
              <span>在线隧道:</span>
              <span class="font-extrabold text-toy-black">{{ platform.tunnel_online }}/{{ platform.tunnel_total }}</span>
            </div>
            <div class="flex justify-between text-toy-muted">
              <span>连接数:</span>
              <span v-if="platform.conns_src !== 'local' && platform.conns_src !== 'platform'" class="font-extrabold text-toy-muted">—</span>
              <span v-else class="font-extrabold text-toy-black">{{ platform.conns }}</span>
            </div>
          </div>

          <!-- 在用节点：名称 + 在线时长/分组 + 负载条（与 classic frp-node 同口径） -->
          <div>
            <div class="flex items-center justify-between text-[11px] font-black text-toy-black mb-1">
              <span>在用节点（{{ (platform.nodes || []).length }}）</span>
              <span class="font-mono font-bold text-toy-muted">{{ syncedText(platform.updated_at) }}</span>
            </div>
            <ul v-if="(platform.nodes || []).length" class="space-y-1">
              <li
                v-for="n in (platform.nodes || [])"
                :key="n.name"
                class="flex items-center gap-2 px-2 py-1.5 rounded-lg text-[11px]"
                :class="n.online ? 'bg-toy-bg border border-toy-black/15' : 'border border-dashed border-toy-black/40 opacity-60'"
              >
                <span class="w-1.5 h-1.5 rounded-full shrink-0" :class="n.online ? 'bg-toy-mint' : 'bg-gray-300'"></span>
                <span class="font-bold text-toy-black truncate flex-1 min-w-0" :title="n.name">{{ n.name }}</span>
                <span class="font-mono text-toy-muted shrink-0">
                  <template v-if="!n.online">离线</template>
                  <template v-else>{{ nodeSub(n) }}</template>
                </span>
                <span class="shrink-0 flex items-center gap-1 font-mono" :class="loadToneClass(n.load)">
                  <span class="w-10 h-1.5 rounded-full bg-toy-gray overflow-hidden inline-block">
                    <span class="block h-full rounded-full" :class="loadBarClass(n.load)" :style="{ width: `${loadPct(n.load)}%` }"></span>
                  </span>
                  {{ loadPct(n.load) }}%
                </span>
              </li>
            </ul>
            <p v-else class="text-[11px] font-bold text-toy-muted">该平台暂无隧道挂载节点（隧道建好后自动出现）。</p>
          </div>
        </div>
      </div>

      <!-- 2. 全网汇总条 -->
      <div class="toy-card p-3.5 bg-toy-bg border-2 border-toy-black flex items-center justify-between flex-wrap gap-2 text-xs font-bold font-mono">
        <span>隧道总数: <span class="text-toy-black font-extrabold">{{ frp.summary?.tunnel_total ?? 0 }}</span> · 在线 <span class="text-toy-black font-extrabold">{{ frp.summary?.tunnel_online ?? 0 }}</span><template v-if="offlineTunnels"> · <span class="text-red-600">{{ offlineTunnels }} 条离线</span></template></span>
        <span>当前连接: <span class="text-toy-black font-extrabold">{{ connsSupported ? (frp.summary?.conns ?? 0) : '—' }}</span></span>
        <span class="hidden sm:inline">在用节点: {{ frp.summary?.node_online ?? 0 }}/{{ frp.summary?.node_in_use ?? 0 }}</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { FRPPublicSnapshot } from '../../types/backend';
import { formatBytes, formatUptime } from '../../services/format';

const props = defineProps<{
  frp: FRPPublicSnapshot;
}>();

const LOAD_WARN = 60;
const LOAD_DANGER = 85;

const KIND_LABEL: Record<string, string> = { natfrp: 'Sakura', chmlfrp: 'ChmlFrp', cloudflared: 'Cloudflare' };
function kindLabel(kind: string): string {
  return KIND_LABEL[kind] || '穿透平台';
}

const offlineTunnels = computed(() => {
  const s = props.frp.summary;
  if (!s) return 0;
  return Math.max(0, (s.tunnel_total || 0) - (s.tunnel_online || 0));
});

const nodesOffline = computed(() =>
  Math.max(0, (props.frp.summary?.node_in_use || 0) - (props.frp.summary?.node_online || 0)),
);

// conns_src：platform=平台 API 值；local=面板在 frpc 节点数 socket；
// 空=无数据（未同步/口径不适用，如 Cloudflare 边缘转发）→ 显示「—」
const connsSupported = computed(() =>
  props.frp.platforms.some((p) => p.conns_src === 'local' || p.conns_src === 'platform'),
);

function loadPct(load: number): number {
  return Math.min(100, Math.max(0, Math.round(Number(load) || 0)));
}
function loadToneClass(load: number): string {
  const v = Number(load) || 0;
  if (v >= LOAD_DANGER) return 'text-red-600';
  if (v >= LOAD_WARN) return 'text-toy-orange';
  return 'text-toy-muted';
}
function loadBarClass(load: number): string {
  const v = Number(load) || 0;
  if (v >= LOAD_DANGER) return 'bg-toy-pink';
  if (v >= LOAD_WARN) return 'bg-toy-orange';
  return 'bg-toy-cyan';
}

// ChmlFrp 节点接口不报在线时长（恒 0）→ 退化展示分组，避免「在线 0 天」
function nodeSub(n: { uptime: number; group: string }): string {
  const s = Number(n.uptime) || 0;
  return s > 0 ? `在线 ${formatUptime(s)}` : (n.group || '节点');
}

// 面板侧「上次同步」是 unix 秒；平台数据每 3 分钟回流一次
function syncedText(at: number): string {
  const ts = Number(at) || 0;
  if (!ts) return '尚未同步';
  const ago = Math.max(0, Math.floor(Date.now() / 1000) - ts);
  if (ago < 60) return `同步于 ${ago} 秒前`;
  if (ago < 3600) return `同步于 ${Math.floor(ago / 60)} 分钟前`;
  if (ago < 86400) return `同步于 ${Math.floor(ago / 3600)} 小时前`;
  return `同步于 ${Math.floor(ago / 86400)} 天前`;
}
</script>
