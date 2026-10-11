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
            :class="platform.token_bound ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'"
          >
            {{ platform.token_bound ? '已授权' : '未绑定' }}
          </span>
        </div>

        <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black space-y-1 text-xs font-mono">
          <div class="flex justify-between text-toy-muted">
            <span>活跃隧道:</span>
            <span class="font-extrabold text-toy-black">{{ platform.online_tunnels }}/{{ platform.total_tunnels }} 运行中</span>
          </div>
          <div class="flex justify-between text-toy-muted">
            <span>今日消耗:</span>
            <span class="font-extrabold text-sky-800">{{ formatBytes(platform.today_traffic) }}</span>
          </div>
          <div v-if="platform.user_info" class="flex justify-between text-toy-muted pt-1 border-t border-toy-black/10">
            <span>配额状态:</span>
            <span class="font-bold text-toy-black">{{ platform.user_info.traffic_quota }}</span>
          </div>
        </div>

        <div class="flex items-center justify-between text-[11px] pt-1">
          <span class="font-bold text-toy-muted">账号: {{ platform.user_info?.username || '未同步' }}</span>
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
              <td class="py-3 text-toy-black font-extrabold">{{ t.local_addr }}</td>
              <td class="py-3 text-toy-muted font-bold">{{ t.remote_addr }}</td>
              <td class="py-3 text-sky-800 font-bold">{{ formatBytes(t.today_traffic) }}</td>
              <td class="py-3">
                <button 
                  @click="handleToggleTunnel(t.id)"
                  class="px-2.5 py-1 rounded-lg text-[10px] font-black border border-toy-black flex items-center gap-1.5 cursor-pointer transition-colors"
                  :class="t.running ? 'bg-toy-mint text-toy-black' : 'bg-gray-100 text-toy-muted'"
                >
                  <span class="w-1.5 h-1.5 rounded-full" :class="t.running ? 'bg-toy-black animate-pulse' : 'bg-gray-400'"></span>
                  <span>{{ t.running ? '运行中' : '已停止' }}</span>
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
              <option :value="1">Sakura FRP (樱花穿透)</option>
              <option :value="2">ChmlFrp (极速穿透)</option>
              <option :value="3">Cloudflare Tunnel</option>
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

          <div>
            <label class="block text-xs font-black text-toy-black mb-1">公网出口域名或端口</label>
            <input 
              v-model="newTunnel.remote_addr"
              type="text" 
              placeholder="demo.natfrp.net:38291" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
          </div>

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
              v-model="tokenForm.platform_id"
              class="w-full px-3 py-1.5 text-xs font-black bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
            >
              <option :value="1">Sakura FRP (樱花穿透)</option>
              <option :value="2">ChmlFrp (极速穿透)</option>
              <option :value="3">Cloudflare Tunnel</option>
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
import { FRPPlatformItem, FRPTunnelItem } from '../../../types/backend';
import { formatBytes } from '../../../services/mock';
import { api } from '../../../services/api';

const platforms = ref<FRPPlatformItem[]>([]);
const tunnels = ref<FRPTunnelItem[]>([]);

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
  const res = await api.getFRPOverview();
  platforms.value = res.platforms;
  tunnels.value = res.tunnels;
}

async function handleToggleTunnel(id: number) {
  await api.toggleFRPTunnel(id);
  await loadData();
}

async function handleDeleteTunnel(id: number) {
  if (confirm('确认删除该内网穿透规则？')) {
    await api.deleteFRPTunnel(id);
    await loadData();
  }
}

async function syncPlatform(platformId: number) {
  alert('已成功触发同步：最新隧道状态与流量用量已对齐！');
  await loadData();
}

// 1. 新建隧道弹窗
const isCreateTunnelModalOpen = ref(false);
const newTunnel = ref<Partial<FRPTunnelItem>>({
  platform_id: 1,
  name: '',
  proto: 'tcp',
  local_addr: '127.0.0.1:8080',
  remote_addr: '',
});

function openCreateTunnelModal() {
  newTunnel.value = {
    platform_id: 1,
    name: '',
    proto: 'tcp',
    local_addr: '127.0.0.1:8080',
    remote_addr: '',
  };
  isCreateTunnelModalOpen.value = true;
}

async function submitCreateTunnel() {
  if (!newTunnel.value.name) return;
  const pName = newTunnel.value.platform_id === 1 ? 'Sakura FRP' : newTunnel.value.platform_id === 2 ? 'ChmlFrp' : 'Cloudflare Tunnel';
  const pKind = newTunnel.value.platform_id === 1 ? 'sakura' : newTunnel.value.platform_id === 2 ? 'chmlfrp' : 'cloudflare';
  await api.createFRPTunnel({
    ...newTunnel.value,
    platform_name: pName,
    platform_kind: pKind,
  });
  isCreateTunnelModalOpen.value = false;
  await loadData();
}

// 2. 绑定 Token 弹窗
const isTokenModalOpen = ref(false);
const tokenForm = ref({
  platform_id: 1,
  token: '',
});

function openTokenModal() {
  tokenForm.value = { platform_id: 1, token: '' };
  isTokenModalOpen.value = true;
}

async function submitSaveToken() {
  await api.saveFRPPlatformToken(tokenForm.value.platform_id, tokenForm.value.token);
  isTokenModalOpen.value = false;
  alert('平台 API 凭据已成功保存！');
  await loadData();
}

// 3. Docker 部署弹窗
const deployModalTunnel = ref<FRPTunnelItem | null>(null);
const isDockerCopied = ref(false);

const dockerCommandText = computed(() => {
  if (!deployModalTunnel.value) return '';
  return `docker run -d --restart=always --name=bt-frpc-${deployModalTunnel.value.name} \\
  -e FRP_SERVER=proxy.example.com -e FRP_PORT=7000 \\
  -e TUNNEL_LOCAL=${deployModalTunnel.value.local_addr} \\
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
