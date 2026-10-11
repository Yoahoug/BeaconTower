<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>内网穿透托管</span>
          <span class="toy-badge bg-toy-yellow text-[10px]">多平台聚合</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">Sakura FRP · ChmlFrp · Cloudflare Tunnel 集中编排与容器托管</p>
      </div>

      <div class="flex items-center gap-2">
        <button 
          @click="openTokenModal"
          class="toy-btn px-3 py-1.5 bg-white hover:bg-toy-bg text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
        >
          <span>🔑</span>
          <span>绑定平台凭据</span>
        </button>
        <button 
          @click="openCreateTunnelModal"
          class="toy-btn px-3.5 py-1.5 bg-toy-yellow text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
        >
          <span>➕</span>
          <span>新建穿透隧道</span>
        </button>
      </div>
    </div>

    <!-- 错误/提示条 -->
    <div v-if="errorMsg" class="toy-card p-3 bg-red-50 border-2.5 border-red-400 flex items-center justify-between gap-3">
      <span class="text-xs font-bold text-red-700">⚠ {{ errorMsg }}</span>
      <button @click="errorMsg = ''" class="text-xs font-black cursor-pointer">✕</button>
    </div>
    <div v-if="notice" class="toy-card p-3 bg-toy-mint-light border-2.5 border-toy-black flex items-center justify-between gap-3">
      <span class="text-xs font-bold text-toy-black">✓ {{ notice }}</span>
      <button @click="notice = ''" class="text-xs font-black cursor-pointer">✕</button>
    </div>

    <!-- 1. 三大平台状态卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-3.5">
      <div 
        v-for="platform in platforms" 
        :key="platform.id"
        class="toy-card p-4 bg-white flex flex-col justify-between space-y-3"
      >
        <div class="flex items-start justify-between">
          <div class="flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-yellow/30 flex items-center justify-center font-black text-xs">
              {{ platform.kind.slice(0, 2).toUpperCase() }}
            </div>
            <div>
              <h4 class="font-black text-sm text-toy-black">{{ platform.name }}</h4>
              <span class="text-[10px] font-bold text-toy-muted uppercase font-mono">{{ platform.kind }}</span>
            </div>
          </div>
          <span 
            class="toy-badge text-[10px]"
            :class="platform.status === 'ok' ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'"
          >
            {{ platform.status === 'ok' ? '同步正常' : platform.status === 'pending' ? '待同步' : '异常' }}
          </span>
        </div>

        <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black space-y-1 text-xs font-mono">
          <div class="flex justify-between text-toy-muted">
            <span>活跃隧道:</span>
            <span class="font-extrabold text-toy-black">{{ platform.tunnel_online }}/{{ platform.tunnel_total }} 运行中</span>
          </div>
          <div class="flex justify-between text-toy-muted">
            <span>今日消耗:</span>
            <span class="font-extrabold text-sky-800">{{ formatBytes(platform.traffic_day_used) }}</span>
          </div>
          <div class="flex justify-between text-toy-muted pt-1 border-t border-toy-black/10">
            <span>账号:</span>
            <span class="font-bold text-toy-black">{{ platform.username || "未同步" }}</span>
          </div>
          <div v-if="platform.traffic_remain > 0" class="flex justify-between text-toy-muted">
            <span>剩余流量:</span>
            <span class="font-bold text-toy-black">{{ formatBytes(platform.traffic_remain) }}</span>
          </div>
        </div>

        <div class="flex items-center justify-between text-[11px] pt-1">
          <span class="font-bold text-toy-muted">连接数: {{ platform.conns }}</span>
          <button 
            @click="syncPlatform(platform.id)"
            class="text-toy-black font-black hover:text-sky-700 hover:underline cursor-pointer"
          >
            立即同步 ↺
          </button>
        </div>
      </div>
    </div>

    <!-- 2. 跨平台合并隧道管理列表 -->
    <div class="toy-card p-5 bg-white space-y-4">
      <!-- 过滤页签 + 搜索 -->
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div class="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0 scrollbar-none">
          <button 
            v-for="tab in filterTabs" 
            :key="tab.key"
            @click="activeTab = tab.key"
            class="px-3 py-1 rounded-lg text-xs font-black border-2 border-toy-black transition-all shrink-0 cursor-pointer"
            :class="activeTab === tab.key ? 'bg-toy-black text-white shadow-toy-sm' : 'bg-white text-toy-black hover:bg-toy-yellow/30'"
          >
            {{ tab.label }}
          </button>
        </div>

        <span class="text-xs font-mono text-toy-muted font-bold">共 {{ filteredTunnels.length }} 条穿透规则</span>
      </div>

      <!-- 隧道表格 -->
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
              <th class="pb-2">隧道名称 / 平台</th>
              <th class="pb-2">协议</th>
              <th class="pb-2">内网映射端点</th>
              <th class="pb-2">公网出口域名/端口</th>
              <th class="pb-2">今日流量</th>
              <th class="pb-2">状态 / 开关</th>
              <th class="pb-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-toy-gray font-mono">
            <tr v-for="t in filteredTunnels" :key="t.id" class="hover:bg-toy-bg/50 transition-colors">
              <td class="py-3 font-bold font-sans text-toy-black">
                <div>{{ t.name }}</div>
                <span class="px-1.5 py-0.2 text-[9px] font-bold bg-toy-yellow/50 border border-toy-black rounded text-toy-black">
                  {{ t.platform_name }}
                </span>
              </td>
              <td class="py-3">
                <span class="px-1.5 py-0.5 rounded text-[10px] font-black border border-toy-black bg-white">
                  {{ t.proto.toUpperCase() }}
                </span>
              </td>
              <td class="py-3 text-toy-black font-extrabold">{{ t.local_ip }}:{{ t.local_port }}</td>
              <td class="py-3 text-toy-muted font-bold">{{ t.remote || '—' }}</td>
              <td class="py-3 text-sky-800 font-bold">{{ formatBytes(t.today_up + t.today_down) }}</td>
              <td class="py-3">
                <button 
                  @click="handleToggleTunnel(t)"
                  class="px-2.5 py-1 rounded-lg text-[10px] font-black border border-toy-black flex items-center gap-1.5 cursor-pointer transition-colors"
                  :class="t.online ? 'bg-toy-mint text-toy-black' : 'bg-gray-100 text-toy-muted'"
                >
                  <span class="w-1.5 h-1.5 rounded-full" :class="t.online ? 'bg-toy-black animate-pulse' : 'bg-gray-400'"></span>
                  <span>{{ t.online ? '运行中' : '已停止' }}</span>
                </button>
              </td>
              <td class="py-3 text-right space-x-1.5 font-sans">
                <button 
                  @click="openDeployDockerModal(t)"
                  class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-toy-yellow transition-colors cursor-pointer"
                  title="生成 Docker 容器部署指令"
                >
                  Docker
                </button>
                <button 
                  @click="handleDeleteTunnel(t.id)"
                  class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-red-50 text-red-600 transition-colors cursor-pointer"
                >
                  删除
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 弹窗 1: 新建穿透隧道弹窗 -->
    <div v-if="isCreateTunnelModalOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div 
        class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 relative overflow-hidden"
        :style="{ boxShadow: '6px 6px 0px #121212' }"
      >
        <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="text-xl">➕</span>
            <div>
              <h3 class="text-base font-black text-toy-black">新建内网穿透隧道</h3>
              <p class="text-[10px] font-bold text-toy-muted">将内网服务暴露至公网访问</p>
            </div>
          </div>
          <button 
            @click="isCreateTunnelModalOpen = false"
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
          >
            ✕
          </button>
        </div>

        <form @submit.prevent="submitCreateTunnel" class="mt-4 space-y-3.5">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">目标穿透平台</label>
            <select 
              v-model="newTunnel.platform_id"
              class="w-full px-3 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
            >
              <option v-for="p in platforms" :key="p.id" :value="p.id">{{ p.name }} ({{ p.kind }})</option>
            </select>
          </div>

          <div>
            <label class="block text-xs font-black text-toy-black mb-1">隧道标识名称</label>
            <input 
              v-model="newTunnel.name"
              type="text" 
              placeholder="例如: nas-web-drive" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
              required
            />
          </div>

          <div class="grid grid-cols-2 gap-2">
            <div>
              <label class="block text-xs font-black text-toy-black mb-1">传输协议</label>
              <select 
                v-model="newTunnel.proto"
                class="w-full px-3 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
              >
                <option value="tcp">TCP</option>
                <option value="http">HTTP</option>
                <option value="https">HTTPS</option>
                <option value="udp">UDP</option>
              </select>
            </div>
            <div>
              <label class="block text-xs font-black text-toy-black mb-1">内网目标地址</label>
              <input 
                v-model="newTunnel.local_addr"
                type="text" 
                placeholder="127.0.0.1:8080" 
                class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
                required
              />
            </div>
          </div>

          <p class="text-[10px] font-bold text-toy-muted">公网出口由平台分配（ChmlFrp 隧道名仅限字母/数字/下划线）</p>

          <div class="pt-2 flex items-center justify-end gap-2">
            <button 
              type="button" 
              @click="isCreateTunnelModalOpen = false"
              class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer"
            >
              取消
            </button>
            <button 
              type="submit" 
              class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer"
            >
              确定创建
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- 弹窗 2: 绑定平台 Token 弹窗 -->
    <div v-if="isTokenModalOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div 
        class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-6 relative overflow-hidden"
        :style="{ boxShadow: '6px 6px 0px #121212' }"
      >
        <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="text-xl">🔑</span>
            <div>
              <h3 class="text-base font-black text-toy-black">绑定平台 API Token</h3>
              <p class="text-[10px] font-bold text-toy-muted">自动同步上游隧道状态</p>
            </div>
          </div>
          <button 
            @click="isTokenModalOpen = false"
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
          >
            ✕
          </button>
        </div>

        <form @submit.prevent="submitSaveToken" class="mt-4 space-y-3.5">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">选择平台</label>
            <select 
              v-model="tokenForm.kind"
              class="w-full px-3 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
            >
              <option value="sakura">Sakura FRP (访问密钥)</option>
              <option value="cloudflare">Cloudflare (API Token)</option>
            </select>
          </div>

          <div>
            <label class="block text-xs font-black text-toy-black mb-1">API Token / Access Key</label>
            <input 
              v-model="tokenForm.token"
              type="password" 
              placeholder="请输入平台的 API 令牌密钥..." 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
          </div>

          <div class="pt-2 flex items-center justify-end gap-2">
            <button 
              type="button" 
              @click="isTokenModalOpen = false"
              class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer"
            >
              取消
            </button>
            <button 
              type="submit" 
              class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer"
            >
              保存并授权
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- 弹窗 3: Docker 客户端部署代码复制弹窗 -->
    <div v-if="deployModalTunnel" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div 
        class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 relative overflow-hidden"
        :style="{ boxShadow: '6px 6px 0px #121212' }"
      >
        <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="text-xl">🐳</span>
            <div>
              <h3 class="text-base font-black text-toy-black">Docker 自动化拉起指令</h3>
              <p class="text-[10px] font-bold text-toy-muted">{{ deployModalTunnel.name }} 部署脚本</p>
            </div>
          </div>
          <button 
            @click="deployModalTunnel = null"
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
          >
            ✕
          </button>
        </div>

        <div class="mt-4 space-y-3">
          <p class="text-xs font-bold text-toy-muted">在目标服务器终端直接运行以下指令，即可以后台容器拉起该穿透隧道：</p>
          <div class="relative">
            <textarea 
              readonly
              :value="dockerCommandText"
              rows="4"
              class="w-full p-3 bg-toy-bg border-2 border-toy-black rounded-xl text-xs font-mono font-bold focus:outline-none"
            ></textarea>
          </div>

          <div class="flex items-center justify-end gap-2 pt-1">
            <button 
              @click="copyDockerCommand"
              class="toy-btn px-4 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer"
            >
              {{ isDockerCopied ? '已复制 ✔' : '复制命令' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import type { FRPTunnelItem } from '../../../types/backend';
import { formatBytes } from '../../../services/format';
import { api } from '../../../services/api';

const platforms = ref<import('../../../types/backend').FRPPlatformItem[]>([]);
const tunnels = ref<FRPTunnelItem[]>([]);
const errorMsg = ref('');
const notice = ref('');
const busy = ref(false);

const activeTab = ref('all');
const filterTabs = [
  { key: 'all', label: '全部平台' },
  { key: 'sakura', label: 'Sakura FRP' },
  { key: 'chmlfrp', label: 'ChmlFrp' },
  { key: 'cloudflare', label: 'Cloudflare' },
];

const filteredTunnels = computed(() => {
  if (activeTab.value === 'all') return tunnels.value;
  return tunnels.value.filter(t => t.platform_kind === activeTab.value);
});

async function loadData() {
  try {
    const res = await api.getFRPOverview();
    platforms.value = res.platforms;
    tunnels.value = res.tunnels;
    errorMsg.value = '';
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '读取穿透概览失败';
  }
}

// 风格内确认弹窗（替代原生 confirm）
const confirmText = ref('');
let confirmAction: (() => void | Promise<void>) | null = null;
function confirmYes() {
  confirmText.value = '';
  const a = confirmAction;
  confirmAction = null;
  if (a) void a();
}

async function handleToggleTunnel(t: FRPTunnelItem) {
  // 后端隧道语义：online=true 想下线 → offline（Sakura 强制下线）；
  // 已停止想上线 → 各平台 API 未开放重启，如实提示而非假装成功
  if (!t.online) {
    errorMsg.value = '面板暂不支持一键重启隧道（平台 API 未开放），请删除后重建或到平台控制台操作';
    return;
  }
  confirmText.value = `确认强制下线隧道「${t.name}」？外网访问将立即中断。`;
  confirmAction = async () => {
    try {
      await api.toggleFRPTunnel(t.id, true);
      await loadData();
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '下线失败';
    }
  };
}

function handleDeleteTunnel(id: number) {
  confirmText.value = '确认删除该内网穿透规则？此操作会同时删除平台侧的隧道。';
  confirmAction = async () => {
    try {
      await api.deleteFRPTunnel(id);
      await loadData();
    } catch (e) {
      errorMsg.value = e instanceof Error ? e.message : '删除失败';
    }
  };
}

async function syncPlatform(platformId: number) {
  busy.value = true;
  try {
    await api.syncFRPPlatform(platformId);
    notice.value = '同步完成：最新隧道状态与流量用量已对齐';
    await loadData();
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '同步失败';
  } finally {
    busy.value = false;
  }
}

// 1. 新建隧道弹窗
const isCreateTunnelModalOpen = ref(false);
const newTunnel = ref({
  platform_id: 0,
  name: '',
  proto: 'tcp',
  local_addr: '127.0.0.1:8080',
});

function openCreateTunnelModal() {
  newTunnel.value = {
    platform_id: platforms.value[0]?.id ?? 0,
    name: '',
    proto: 'tcp',
    local_addr: '127.0.0.1:8080',
  };
  isCreateTunnelModalOpen.value = true;
}

async function submitCreateTunnel() {
  if (!newTunnel.value.name) return;
  if (!newTunnel.value.platform_id) {
    errorMsg.value = '请先绑定至少一个穿透平台';
    return;
  }
  try {
    await api.createFRPTunnel({ ...newTunnel.value, platform_id: newTunnel.value.platform_id });
    isCreateTunnelModalOpen.value = false;
    await loadData();
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '创建隧道失败';
  }
}

// 2. 绑定 Token 弹窗（真实：Sakura/Cloudflare 走静态令牌，ChmlFrp 走设备码授权）
const isTokenModalOpen = ref(false);
const tokenForm = ref({
  kind: 'sakura',
  token: '',
});

function openTokenModal() {
  tokenForm.value = { kind: 'sakura', token: '' };
  isTokenModalOpen.value = true;
}

async function submitSaveToken() {
  try {
    await api.saveFRPPlatformToken({ kind: tokenForm.value.kind }, tokenForm.value.token);
    isTokenModalOpen.value = false;
    notice.value = '平台凭据已绑定并完成首次同步';
    await loadData();
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '绑定失败';
  }
}

// 3. Docker 部署弹窗
const deployModalTunnel = ref<FRPTunnelItem | null>(null);
const isDockerCopied = ref(false);

const dockerCommandText = computed(() => {
  if (!deployModalTunnel.value) return '';
  return `docker run -d --restart=always --name=bt-frpc-${deployModalTunnel.value.name} \\
  -e FRP_SERVER=proxy.example.com -e FRP_PORT=7000 \\
  -e TUNNEL_LOCAL=${deployModalTunnel.value.local_ip}:${deployModalTunnel.value.local_port} \\
  natfrp/frpc:latest`;
});

function openDeployDockerModal(t: FRPTunnelItem) {
  deployModalTunnel.value = t;
  isDockerCopied.value = false;
}

function copyDockerCommand() {
  navigator.clipboard.writeText(dockerCommandText.value);
  isDockerCopied.value = true;
  setTimeout(() => { isDockerCopied.value = false; }, 2000);
}

onMounted(() => {
  loadData();
});
</script>
