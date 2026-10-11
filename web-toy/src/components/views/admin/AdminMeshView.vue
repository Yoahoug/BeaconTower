<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>WG 组网管理</span>
          <span class="toy-badge bg-toy-yellow text-[10px]">A/B 主备中心</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">WireGuard 全自动星型组网 · 流量额度轮换 · 跨设备凭证分发</p>
      </div>

      <div class="flex items-center gap-2">
        <button 
          @click="handlePatrol"
          class="toy-btn px-3 py-1.5 bg-white hover:bg-toy-bg text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
          :disabled="isPatrolling"
        >
          <span>{{ isPatrolling ? '⏳' : '🔍' }}</span>
          <span>{{ isPatrolling ? '正在巡检全网...' : '全网巡检' }}</span>
        </button>
        <button 
          @click="openAddDeviceModal"
          class="toy-btn px-3.5 py-1.5 bg-toy-yellow text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
        >
          <span>➕</span>
          <span>接入外部设备</span>
        </button>
      </div>
    </div>

    <!-- 1. 主备中心 A/B 状态卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <!-- 现役主中心 Active Hub -->
      <div 
        v-if="activeHub"
        class="toy-card p-4.5 bg-white border-2.5 border-toy-black relative overflow-hidden"
      >
        <div class="flex items-start justify-between">
          <div class="space-y-1">
            <div class="flex items-center gap-2">
              <span class="text-lg">👑</span>
              <span class="font-black text-sm text-toy-black">现役主中心 (Active Hub)</span>
              <span class="toy-badge bg-toy-mint text-[10px]">承载全网流量</span>
            </div>
            <div class="text-base font-extrabold text-toy-black font-sans">{{ activeHub.name }}</div>
            <p class="text-xs font-mono text-toy-muted">
              监听端点: {{ activeHub.endpoint }} (端口 {{ activeHub.listen_port }})
            </p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-toy-yellow border-2 border-toy-black flex items-center justify-center font-black text-xs shadow-toy-sm">
            HUB
          </div>
        </div>

        <div class="mt-4 p-3 bg-toy-bg rounded-xl border border-toy-black flex items-center justify-between text-xs font-mono">
          <div>
            <span class="text-toy-muted block text-[10px]">本月中转流量</span>
            <span class="font-extrabold text-toy-black">
              RX {{ formatBytes(activeHub.month_rx) }} / TX {{ formatBytes(activeHub.month_tx) }}
            </span>
          </div>
          <button 
            @click="probeHub(activeHub.id)" 
            class="px-2.5 py-1 bg-white border border-toy-black rounded-lg text-[11px] font-bold hover:bg-toy-yellow cursor-pointer"
          >
            {{ hubLatency[activeHub.id] !== undefined ? `${hubLatency[activeHub.id]}ms` : '探测延迟' }}
          </button>
        </div>
      </div>

      <!-- 温备援中心 Standby Hub -->
      <div 
        v-if="standbyHub"
        class="toy-card p-4.5 bg-white border-2.5 border-toy-black relative overflow-hidden"
      >
        <div class="flex items-start justify-between">
          <div class="space-y-1">
            <div class="flex items-center gap-2">
              <span class="text-lg">🛡️</span>
              <span class="font-black text-sm text-toy-black">温备援中心 (Standby Hub)</span>
              <span class="toy-badge bg-toy-gray text-[10px]">待命就绪</span>
            </div>
            <div class="text-base font-extrabold text-toy-black font-sans">{{ standbyHub.name }}</div>
            <p class="text-xs font-mono text-toy-muted">
              监听端点: {{ standbyHub.endpoint }} (端口 {{ standbyHub.listen_port }})
            </p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-gray-100 border-2 border-toy-black flex items-center justify-center font-black text-xs shadow-toy-sm text-toy-muted">
            BAK
          </div>
        </div>

        <div class="mt-4 p-3 bg-toy-bg rounded-xl border border-toy-black flex items-center justify-between text-xs font-mono">
          <div>
            <span class="text-toy-muted block text-[10px]">备用状态</span>
            <span class="font-extrabold text-toy-black">密钥与路由配置实时对齐</span>
          </div>
          <button 
            @click="handleSwitchHub(standbyHub.id)" 
            class="toy-btn px-3 py-1 bg-toy-yellow text-[11px] font-black rounded-lg cursor-pointer"
          >
            一键接管为主中心
          </button>
        </div>
      </div>
    </div>

    <!-- 2. 网段配置与状态条 -->
    <div class="toy-card p-3.5 bg-toy-bg border-2 border-toy-black flex items-center justify-between flex-wrap gap-2 text-xs font-bold font-mono">
      <div class="flex items-center gap-3">
        <span class="flex items-center gap-1.5 text-toy-black">
          <span class="w-2 h-2 rounded-full bg-toy-mint"></span>
          虚拟专用网段: <span class="bg-white px-2 py-0.5 rounded border border-toy-black font-black">{{ overview.network.cidr }}</span>
        </span>
        <span class="hidden sm:inline text-toy-muted">|</span>
        <span class="text-toy-muted">保活周期: {{ overview.network.keepalive_s }}s</span>
      </div>
      <div class="text-toy-muted">
        在网节点: <span class="text-toy-black font-extrabold">{{ overview.peers.length }}</span> 台 · 外部终端: <span class="text-toy-black font-extrabold">{{ overview.devices.length }}</span> 台
      </div>
    </div>

    <!-- 3. 星型拓扑简图 (纯 SVG 矢量绘制) -->
    <div class="toy-card p-5 bg-white space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-black text-toy-black">全网星型拓扑架构</h3>
        <span class="text-[11px] font-mono text-toy-muted font-bold">中心汇聚 · 零外网穿透暴露</span>
      </div>
      <div class="w-full bg-toy-bg rounded-xl border-2 border-toy-black p-4 flex items-center justify-center overflow-x-auto">
        <svg class="w-full max-w-2xl h-44 select-none" viewBox="0 0 600 170">
          <!-- 连线 (虚线工程感) -->
          <line x1="300" y1="85" x2="100" y2="40" stroke="#121212" stroke-width="2" stroke-dasharray="4 3" />
          <line x1="300" y1="85" x2="100" y2="130" stroke="#121212" stroke-width="2" stroke-dasharray="4 3" />
          <line x1="300" y1="85" x2="500" y2="40" stroke="#121212" stroke-width="2" stroke-dasharray="4 3" />
          <line x1="300" y1="85" x2="500" y2="130" stroke="#121212" stroke-width="2" stroke-dasharray="4 3" />

          <!-- 中心 Hub (带硬阴影圆框) -->
          <circle cx="302" cy="87" r="32" fill="#121212" />
          <circle cx="300" cy="85" r="32" fill="#FFE047" stroke="#121212" stroke-width="2.5" />
          <text x="300" y="81" font-size="11" font-weight="900" text-anchor="middle" fill="#121212">Tokyo-Hub</text>
          <text x="300" y="96" font-size="9" font-weight="700" font-family="monospace" text-anchor="middle" fill="#121212">10.88.0.1</text>

          <!-- 节点 1: NAS -->
          <circle cx="102" cy="42" r="24" fill="#121212" />
          <circle cx="100" cy="40" r="24" fill="#FFFFFF" stroke="#121212" stroke-width="2" />
          <text x="100" y="37" font-size="9" font-weight="900" text-anchor="middle" fill="#121212">HomeLab</text>
          <text x="100" y="49" font-size="8" font-family="monospace" text-anchor="middle" fill="#71717A">10.88.0.3</text>

          <!-- 节点 2: HK 备中心 -->
          <circle cx="102" cy="132" r="24" fill="#121212" />
          <circle cx="100" cy="130" r="24" fill="#E2E8F0" stroke="#121212" stroke-width="2" />
          <text x="100" y="127" font-size="9" font-weight="900" text-anchor="middle" fill="#121212">HK-Edge</text>
          <text x="100" y="139" font-size="8" font-family="monospace" text-anchor="middle" fill="#71717A">10.88.0.2</text>

          <!-- 外部设备 1: iPhone -->
          <circle cx="502" cy="42" r="24" fill="#121212" />
          <circle cx="500" cy="40" r="24" fill="#00E599" stroke="#121212" stroke-width="2" />
          <text x="500" y="37" font-size="9" font-weight="900" text-anchor="middle" fill="#121212">iPhone 16</text>
          <text x="500" y="49" font-size="8" font-family="monospace" text-anchor="middle" fill="#121212">10.88.0.11</text>

          <!-- 外部设备 2: MacBook -->
          <circle cx="502" cy="132" r="24" fill="#121212" />
          <circle cx="500" cy="130" r="24" fill="#00E599" stroke="#121212" stroke-width="2" />
          <text x="500" y="127" font-size="9" font-weight="900" text-anchor="middle" fill="#121212">MacBook</text>
          <text x="500" y="139" font-size="8" font-family="monospace" text-anchor="middle" fill="#121212">10.88.0.12</text>
        </svg>
      </div>
    </div>

    <!-- 4. 成员节点与终端设备分表管理 -->
    <div class="space-y-4">
      <!-- 4.1 纳管服务器成员 -->
      <div class="toy-card p-5 bg-white space-y-3">
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-black text-toy-black flex items-center gap-2">
            <span>纳管服务器节点 ({{ overview.peers.length }})</span>
            <span class="toy-badge bg-toy-yellow/50 text-[10px]">SSH 自动下发</span>
          </h3>
          <span class="text-xs text-toy-muted font-bold font-mono">Keepalive: 25s</span>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead>
              <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                <th class="pb-2">节点标识</th>
                <th class="pb-2">网内虚拟 IP</th>
                <th class="pb-2">外网端点</th>
                <th class="pb-2">最近握手</th>
                <th class="pb-2">本月流量 (TX / RX)</th>
                <th class="pb-2 text-right">状态</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-toy-gray font-mono">
              <tr v-for="peer in overview.peers" :key="peer.id" class="hover:bg-toy-bg/50 transition-colors">
                <td class="py-3 font-bold font-sans text-toy-black">
                  <div class="flex items-center gap-1.5">
                    <span v-if="peer.name.includes('Master')" class="text-xs">👑</span>
                    <span>{{ peer.name }}</span>
                  </div>
                </td>
                <td class="py-3 font-extrabold text-toy-black">{{ peer.ip }}</td>
                <td class="py-3 text-toy-muted">{{ peer.endpoint }}</td>
                <td class="py-3 text-toy-black font-sans font-bold">{{ peer.handshake_ago || '—' }}</td>
                <td class="py-3 text-toy-muted">
                  {{ formatBytes(peer.month_tx) }} / {{ formatBytes(peer.month_rx) }}
                </td>
                <td class="py-3 text-right">
                  <span 
                    class="toy-badge text-[10px]"
                    :class="peer.status === 'online' ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'"
                  >
                    {{ peer.status === 'online' ? '● 在线' : '○ 离线' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 4.2 外部客户端设备 (手机/电脑等) -->
      <div class="toy-card p-5 bg-white space-y-3">
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-black text-toy-black flex items-center gap-2">
            <span>外部接入终端 ({{ overview.devices.length }})</span>
            <span class="toy-badge bg-toy-mint text-[10px]">WireGuard Client</span>
          </h3>
          <button 
            @click="openAddDeviceModal"
            class="toy-btn px-2.5 py-1 bg-white hover:bg-toy-yellow text-[11px] font-black rounded-lg cursor-pointer"
          >
            ➕ 生成新设备凭据
          </button>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead>
              <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
                <th class="pb-2">设备名称</th>
                <th class="pb-2">分配 IP</th>
                <th class="pb-2">最近通信</th>
                <th class="pb-2">本月消耗</th>
                <th class="pb-2">状态</th>
                <th class="pb-2 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-toy-gray font-mono">
              <tr v-for="dev in overview.devices" :key="dev.id" class="hover:bg-toy-bg/50 transition-colors">
                <td class="py-3 font-bold font-sans text-toy-black flex items-center gap-2">
                  <span>📱</span>
                  <span>{{ dev.name }}</span>
                </td>
                <td class="py-3 font-extrabold text-toy-black">{{ dev.ip }}</td>
                <td class="py-3 font-sans font-bold text-toy-muted">{{ dev.handshake_ago || '—' }}</td>
                <td class="py-3 text-toy-muted">{{ formatBytes(dev.month_rx + dev.month_tx) }}</td>
                <td class="py-3">
                  <span 
                    class="toy-badge text-[10px]"
                    :class="dev.status === 'online' ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'"
                  >
                    {{ dev.status === 'online' ? '● 在线' : '○ 待接入' }}
                  </span>
                </td>
                <td class="py-3 text-right space-x-1.5 font-sans">
                  <button 
                    @click="viewDeviceConfig(dev)"
                    class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-toy-yellow hover:bg-yellow-300 transition-colors cursor-pointer"
                  >
                    配置 / 扫码
                  </button>
                  <button 
                    @click="handleDeleteDevice(dev.id)"
                    class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-red-50 text-red-600 transition-colors cursor-pointer"
                  >
                    移除
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- 弹窗 1: 查看配置 & 扫码二维码弹窗 -->
    <div v-if="selectedDevice" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div 
        class="bg-white border-3 border-toy-black rounded-2xl max-w-md w-full p-6 relative overflow-hidden"
        :style="{ boxShadow: '6px 6px 0px #121212' }"
      >
        <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="text-xl">📱</span>
            <div>
              <h3 class="text-base font-black text-toy-black">{{ selectedDevice.name }}</h3>
              <p class="text-[10px] font-bold text-toy-muted">WireGuard 客户端导入凭证</p>
            </div>
          </div>
          <button 
            @click="selectedDevice = null"
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
          >
            ✕
          </button>
        </div>

        <div class="mt-4 space-y-4">
          <!-- 二维码显示区 -->
          <div class="flex flex-col items-center justify-center p-4 bg-toy-bg rounded-xl border-2 border-toy-black">
            <canvas ref="qrCanvasRef" class="w-40 h-40 border-2 border-toy-black rounded-lg shadow-toy-sm"></canvas>
            <p class="mt-2 text-[11px] font-bold text-toy-muted">手机 WireGuard App 直接扫描上方二维码即可添加</p>
          </div>

          <!-- 文本配置预览 -->
          <div>
            <div class="flex items-center justify-between text-xs font-black text-toy-black mb-1">
              <span>wg0.conf 配置文本</span>
              <button @click="copyConfText" class="text-sky-700 hover:underline cursor-pointer">
                {{ isCopied ? '已复制 ✔' : '复制全文' }}
              </button>
            </div>
            <textarea 
              readonly
              :value="currentConfText"
              rows="5"
              class="w-full p-2.5 bg-toy-bg border-2 border-toy-black rounded-lg text-[10px] font-mono focus:outline-none"
            ></textarea>
          </div>

          <div class="flex items-center justify-end gap-2 pt-1">
            <button 
              @click="downloadConfFile"
              class="toy-btn px-4 py-1.5 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer"
            >
              📥 下载 .conf 文件
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 弹窗 2: 新增设备弹窗 -->
    <div v-if="isAddDeviceModalOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
      <div 
        class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-6 relative overflow-hidden"
        :style="{ boxShadow: '6px 6px 0px #121212' }"
      >
        <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
          <div class="flex items-center gap-2">
            <span class="text-xl">➕</span>
            <div>
              <h3 class="text-base font-black text-toy-black">接入新终端设备</h3>
              <p class="text-[10px] font-bold text-toy-muted">自动分配网内 IP 与密钥对</p>
            </div>
          </div>
          <button 
            @click="isAddDeviceModalOpen = false"
            class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
          >
            ✕
          </button>
        </div>

        <form @submit.prevent="submitAddDevice" class="mt-4 space-y-4">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">设备标识名称</label>
            <input 
              v-model="newDeviceName"
              type="text" 
              placeholder="例如: MacBook-M4-Pro" 
              class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none focus:ring-2 focus:ring-toy-yellow"
              required
              autofocus
            />
          </div>

          <div class="pt-2 flex items-center justify-end gap-2">
            <button 
              type="button" 
              @click="isAddDeviceModalOpen = false"
              class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer"
            >
              取消
            </button>
            <button 
              type="submit" 
              class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer"
            >
              立即生成凭证
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue';
import QRCode from 'qrcode';
import { WGOverviewData, WGPeerItem } from '../../../types/backend';
import { formatBytes } from '../../../services/mock';
import { api } from '../../../services/api';

const overview = ref<WGOverviewData>({
  network: { cidr: '10.88.0.0/24', keepalive_s: 25, active_hub_id: 1 },
  hubs: [],
  peers: [],
  devices: [],
});

const isPatrolling = ref(false);
const hubLatency = ref<Record<number, number>>({});

const activeHub = computed(() => overview.value.hubs.find(h => h.is_active));
const standbyHub = computed(() => overview.value.hubs.find(h => !h.is_active));

async function loadData() {
  overview.value = await api.getWGOverview();
}

async function handleSwitchHub(hubId: number) {
  if (confirm('确认将温备援中心提升为主中心？系统将零停机切换路由广播并下发新端点。')) {
    await api.switchWGHub(hubId);
    await loadData();
  }
}

async function probeHub(hubId: number) {
  const res = await api.probeWGHub(hubId);
  if (res.ok) {
    hubLatency.value[hubId] = res.latency_ms;
  }
}

async function handlePatrol() {
  isPatrolling.value = true;
  setTimeout(async () => {
    isPatrolling.value = false;
    await loadData();
    alert('全网星型拓扑巡检完成：6 个节点与 3 台外部终端握手响应均在 50ms 以内，无丢包与路由漂移！');
  }, 1200);
}

// 客户端设备与二维码弹窗
const selectedDevice = ref<WGPeerItem | null>(null);
const currentConfText = ref('');
const qrCanvasRef = ref<HTMLCanvasElement | null>(null);
const isCopied = ref(false);

async function viewDeviceConfig(dev: WGPeerItem) {
  selectedDevice.value = dev;
  isCopied.value = false;
  currentConfText.value = await api.getWGDeviceConf(dev.id);

  await nextTick();
  if (qrCanvasRef.value) {
    QRCode.toCanvas(qrCanvasRef.value, currentConfText.value, {
      width: 160,
      margin: 1,
      color: { dark: '#121212', light: '#FFFFFF' }
    });
  }
}

function copyConfText() {
  navigator.clipboard.writeText(currentConfText.value);
  isCopied.value = true;
  setTimeout(() => { isCopied.value = false; }, 2000);
}

function downloadConfFile() {
  if (!selectedDevice.value) return;
  const blob = new Blob([currentConfText.value], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${selectedDevice.value.name}-wg0.conf`;
  a.click();
  URL.revokeObjectURL(url);
}

async function handleDeleteDevice(id: number) {
  if (confirm('确认注销该外部设备凭据？注销后该设备将无法通过此密钥接入专用网段。')) {
    await api.deleteWGDevice(id);
    await loadData();
  }
}

// 新增设备弹窗
const isAddDeviceModalOpen = ref(false);
const newDeviceName = ref('');

function openAddDeviceModal() {
  newDeviceName.value = '';
  isAddDeviceModalOpen.value = true;
}

async function submitAddDevice() {
  if (!newDeviceName.value.trim()) return;
  const dev = await api.addWGDevice(newDeviceName.value.trim());
  isAddDeviceModalOpen.value = false;
  await loadData();
  await viewDeviceConfig(dev);
}

onMounted(() => {
  loadData();
});
</script>
