<template>
  <div class="min-h-screen bg-toy-bg flex font-sans selection:bg-toy-yellow selection:text-toy-black">
    <!-- 1. 玩具侧边栏 (Toy Sidebar，含左下角两个风格切换坞与管理员切换) -->
    <ToySidebar
      :is-open="isMobileSidebarOpen"
      :collapsed="isSidebarCollapsed"
      :active-key="currentNav"
      :online-count="summary.online"
      :total-count="summary.total"
      :total-watts="summary.watts"
      :avatar-mode="avatarMode"
      :is-admin-logged-in="isAdminLoggedIn"
      @close="isMobileSidebarOpen = false"
      @update:collapsed="isSidebarCollapsed = $event"
      @select="handleNavSelect"
      @update-avatar-mode="handleUpdateAvatarMode"
      @open-theme-modal="isThemeModalOpen = true"
      @toggle-admin="handleToggleAdmin"
    />

    <!-- 2. 主页面内容区域 (动态自适应侧栏收缩: pl-20 或 pl-64，绝无空隙) -->
    <div
      class="flex-1 flex flex-col min-w-0 transition-all duration-200"
      :class="isSidebarCollapsed ? 'lg:pl-20' : 'lg:pl-64'"
    >
      <!-- 2.1 顶部全局导航栏 (Top Navigation Bar) -->
      <header class="bg-white border-b-2.5 border-toy-black sticky top-0 z-30 px-4 sm:px-6 py-2.5 select-none">
        <div class="flex items-center justify-between gap-3">
          <!-- 左侧：移动端侧栏汉堡开关 + 当前板块名称 / 面包屑 -->
          <div class="flex items-center gap-3">
            <button
              @click="isMobileSidebarOpen = true"
              class="lg:hidden w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-yellow flex items-center justify-center font-black text-sm"
              title="打开侧边栏"
            >
              ☰
            </button>
            <div class="flex items-center gap-2">
              <button
                v-if="currentNav === 'server-detail'"
                @click="currentNav = 'overview'"
                class="px-2 py-0.5 rounded text-xs font-black border border-toy-black bg-toy-bg hover:bg-toy-yellow cursor-pointer"
              >
                ◀ 返回总览
              </button>
              <ToyMenuIcons :name="currentNavMeta.iconName" class="w-5 h-5" />
              <h2 class="text-base font-extrabold text-toy-black">{{ currentNavMeta.label }}</h2>
              <span class="hidden sm:inline-block px-1.5 py-0.5 text-[10px] font-bold bg-toy-gray border border-toy-black rounded text-toy-muted">
                {{ currentNavMeta.desc }}
              </span>
            </div>
          </div>

          <!-- 右侧：数据新鲜度 + 实时推流指示 + 管理员状态 -->
          <div class="flex items-center gap-2 sm:gap-3">
            <!-- 数据新鲜度：SSE 断流时显示离线快照时间（classic 顶栏同口径） -->
            <span
              class="hidden sm:inline-flex items-center gap-1 px-2 py-0.5 text-[11px] font-bold border border-toy-black rounded-md"
              :class="stale ? 'bg-toy-yellow/70 text-toy-black' : 'bg-white text-toy-muted'"
              :title="stale ? `实时数据中断，当前展示 ${staleTimeText} 的离线快照` : '距最近一次采集刷新'"
            >
              <span class="w-1.5 h-1.5 rounded-full" :class="stale ? 'bg-toy-orange' : 'bg-toy-mint'"></span>
              {{ stale ? `离线 · ${staleTimeText}` : `更新于 ${formatAgo(secondsSinceUpdate)}` }}
            </span>

            <!-- 状态小标贴 (点击可切换登录/退出) -->
            <button
              @click="handleToggleAdmin"
              class="px-2 py-0.5 text-[11px] font-bold border border-toy-black rounded-md flex items-center gap-1 cursor-pointer transition-colors"
              :class="isAdminLoggedIn ? 'bg-toy-yellow text-toy-black hover:bg-yellow-300' : 'bg-toy-gray/60 text-toy-muted hover:bg-toy-yellow/40 hover:text-toy-black'"
              :title="isAdminLoggedIn ? '点击退出管理员' : '点击登录管理员'"
            >
              <span>{{ isAdminLoggedIn ? '👤 管理员' : '👀 访客模式' }}</span>
            </button>

            <!-- 实时刷新控制按钮（真实后端为 SSE 推流的 pause/resume） -->
            <button
              @click="toggleRealtime"
              class="toy-btn px-2.5 py-1 rounded-lg text-xs flex items-center gap-1.5 cursor-pointer"
              :class="isRealtime ? 'bg-toy-mint text-toy-black' : 'bg-gray-100 text-toy-muted'"
              :title="isRealtime ? '已开启实时同步（SSE 推送）' : '已暂停自动刷新'"
            >
              <span class="w-2 h-2 rounded-full" :class="isRealtime ? 'bg-toy-black animate-ping' : 'bg-gray-400'"></span>
              <span class="font-extrabold">{{ isRealtime ? '实时同步' : '已暂停' }}</span>
            </button>
          </div>
        </div>
      </header>

      <!-- 2.2 页面主内容区域 -->
      <main class="flex-1 p-4 sm:p-6 lg:p-7 space-y-7 sm:space-y-8 max-w-7xl w-full mx-auto">
        <!-- 全局错误条（后端不可达等；失败态另有占位卡，这里只在已有数据时提示） -->
        <div v-if="globalError && servers.length" class="toy-card p-3 bg-red-50 border-2.5 border-red-400 flex items-center justify-between gap-3">
          <span class="text-xs font-black text-red-700">⚠ {{ globalError }}</span>
          <button @click="reloadAll" class="toy-btn px-3 py-1 bg-white text-xs rounded-lg cursor-pointer">重试</button>
        </div>

        <!-- 核心 KPI 积木胶囊 (除子详情页外始终展示全局状态) -->
        <SummaryKpiBar
          v-if="currentNav !== 'server-detail' && !publicFailed && !publicLoading"
          :summary="summary"
          :cost-measured="costMeasured"
          :day-measured="dayMeasured"
        />

        <!-- 公开数据加载 / 失败态（管理页不依赖公开摘要，自管其内部状态） -->
        <template v-if="publicLoading || publicFailed">
          <div v-if="publicLoading" class="toy-card p-5 bg-white space-y-3">
            <div class="h-4 w-40 bg-toy-gray/70 rounded animate-pulse"></div>
            <div class="h-24 bg-toy-gray/40 rounded-xl animate-pulse"></div>
            <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
              <div v-for="i in 3" :key="i" class="h-28 bg-toy-gray/30 rounded-xl animate-pulse"></div>
            </div>
          </div>
          <div v-else class="toy-card p-12 text-center bg-white space-y-3">
            <span class="text-4xl">{{ needLogin ? '🔒' : '⚠️' }}</span>
            <h4 class="text-base font-black text-toy-black">
              {{ needLogin ? '需登录查看' : '数据加载失败' }}
            </h4>
            <p class="text-xs font-bold text-toy-muted">
              {{ needLogin ? (globalError || '私有模式下总览仅管理员可见，登录后继续浏览。') : (globalError || '请检查面板服务与网络连接后重试。') }}
            </p>
            <button
              v-if="needLogin"
              @click="openLogin"
              class="toy-btn px-4 py-2 bg-toy-yellow text-xs font-black rounded-lg mt-2 inline-block cursor-pointer"
            >
              前往登录
            </button>
            <button
              v-else
              @click="reloadAll"
              class="toy-btn px-4 py-2 bg-toy-yellow text-xs font-black rounded-lg mt-2 inline-block cursor-pointer"
            >
              重试
            </button>
          </div>
        </template>

        <!-- 2.2.1 状态总览板块 (Overview) -->
        <template v-else-if="currentNav === 'overview'">
          <!-- 实时波形趋势折线图 -->
          <ToyTrendChart
            :current-up-bps="summary.up_bps"
            :current-down-bps="summary.down_bps"
            :current-watts="summary.watts"
          />

          <!-- WireGuard 与 FRP 穿透微型状态条 -->
          <MeshTunnelStatusBar :wg="wgSnapshot" :frp="frpSnapshot" />

          <!-- 节点列表控制工具条 (筛选标签 + 搜索 + 排序) -->
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-1">
            <!-- 状态过滤 Tab（对齐 classic：全部 / 在线 / 离线） -->
            <div class="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0 scrollbar-none">
              <button
                v-for="tab in filterTabs"
                :key="tab.key"
                @click="currentTab = tab.key"
                class="px-3 py-1.5 rounded-lg text-xs font-black border-2 border-toy-black transition-all shrink-0 cursor-pointer"
                :class="currentTab === tab.key ? 'bg-toy-black text-white shadow-toy-sm' : 'bg-white text-toy-black hover:bg-toy-yellow/30'"
              >
                {{ tab.label }}
                <span
                  class="ml-1 px-1 py-0.2 rounded text-[10px]"
                  :class="currentTab === tab.key ? 'bg-white text-toy-black' : 'bg-toy-gray text-toy-muted'"
                >
                  {{ tab.count }}
                </span>
              </button>
            </div>

            <!-- 搜索与排序 -->
            <div class="flex items-center gap-2">
              <div class="relative min-w-[150px] sm:min-w-[190px]">
                <input
                  v-model="searchQuery"
                  type="text"
                  placeholder="搜索节点 / 地区 / 系统..."
                  class="w-full px-3 py-1.5 text-xs font-bold bg-white border-2 border-toy-black rounded-lg focus:outline-none focus:ring-2 focus:ring-toy-yellow shadow-toy-sm text-toy-black placeholder-toy-muted"
                />
                <span v-if="searchQuery" @click="searchQuery = ''" class="absolute right-2 top-1.5 text-xs cursor-pointer text-toy-muted font-black">✕</span>
              </div>

              <select
                v-model="sortBy"
                class="px-2.5 py-1.5 text-xs font-black bg-white border-2 border-toy-black rounded-lg focus:outline-none shadow-toy-sm cursor-pointer"
              >
                <option value="default">默认排序</option>
                <option value="cpu">CPU 负荷最高</option>
                <option value="mem">内存占用最高</option>
                <option value="power">功耗瓦数最高</option>
              </select>
            </div>
          </div>

          <!-- 服务器节点卡片列表网格 -->
          <div v-if="filteredServers.length > 0" class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            <ServerToyCard
              v-for="server in filteredServers"
              :key="server.id"
              :server="server"
              :avatar-mode="avatarMode"
              :cpu-history="history[server.id]?.cpu || []"
              :power-history="history[server.id]?.power || []"
              :last-updated="secondsSinceUpdate"
              @select-detail="handleSelectServerDetail"
            />
          </div>

          <!-- 空结果提示 -->
          <div v-else class="toy-card p-12 text-center bg-white space-y-3">
            <span class="text-4xl">🔍</span>
            <h4 class="text-base font-black text-toy-black">没有匹配的节点</h4>
            <p class="text-xs text-toy-muted font-bold">可以尝试清空搜索词或切换到「全部节点」标签</p>
            <button @click="resetFilters" class="toy-btn px-4 py-2 bg-toy-yellow text-xs rounded-lg mt-2 inline-block cursor-pointer">
              清除筛选
            </button>
          </div>
        </template>

        <!-- 2.2.2 节点详情视图 (ServerDetailView) -->
        <template v-else-if="currentNav === 'server-detail'">
          <ServerDetailView
            v-if="selectedServer"
            :server="selectedServer"
            :seconds-since-update="secondsSinceUpdate"
            :stale="stale"
            :stale-text="staleTimeText"
            @back="currentNav = 'overview'"
            @need-login="openLogin"
          />
        </template>

        <!-- 2.2.3 穿透状态板块 (TunnelView) -->
        <template v-else-if="currentNav === 'tunnels'">
          <TunnelView :frp="frpSnapshot" />
        </template>

        <!-- 2.2.4 组网状态板块 (MeshPublicView) -->
        <template v-else-if="currentNav === 'mesh'">
          <MeshPublicView :wg="wgSnapshot" />
        </template>

        <!-- 2.2.5 管理面板: 节点管理 -->
        <template v-else-if="currentNav === 'admin-servers'">
          <AdminServersView @refresh="handleRefreshServers" />
        </template>

        <!-- 2.2.6 管理面板: WG 组网管理 -->
        <template v-else-if="currentNav === 'admin-mesh'">
          <AdminMeshView />
        </template>

        <!-- 2.2.7 管理面板: 内网穿透托管 -->
        <template v-else-if="currentNav === 'admin-frp'">
          <AdminFrpView />
        </template>

        <!-- 2.2.8 管理面板: 采集与展示设置 -->
        <template v-else-if="currentNav === 'admin-settings'">
          <AdminSettingsView />
        </template>

        <!-- 2.2.9 管理面板: 安全与账号 -->
        <template v-else-if="currentNav === 'admin-security'">
          <AdminSecurityView />
        </template>

        <!-- 2.2.10 管理面板: 审计日志 -->
        <template v-else-if="currentNav === 'admin-audit'">
          <AdminAuditView />
        </template>
      </main>

      <!-- 2.3 页脚 (Footer) -->
      <footer class="mt-auto border-t-2.5 border-toy-black bg-white px-4 sm:px-6 py-3.5 select-none">
        <div class="flex flex-col sm:flex-row items-center justify-between gap-3 text-xs font-bold text-toy-muted">
          <div class="flex items-center gap-2">
            <span class="text-toy-black font-extrabold">BeaconTower · 信标塔</span>
            <span>·</span>
            <span>自托管服务器监控面板</span>
          </div>
          <div class="flex items-center gap-3">
            <span class="inline-flex items-center gap-1.5" :class="isRealtime ? 'text-toy-black' : 'text-toy-muted'">
              <span class="w-2 h-2 rounded-full" :class="isRealtime ? 'bg-toy-mint animate-pulse' : 'bg-gray-400'"></span>
              {{ isRealtime ? '实时连接正常' : '实时同步已暂停' }}
            </span>
          </div>
        </div>
      </footer>
    </div>

    <!-- 3. 管理员登录弹窗 (Login Modal) -->
    <LoginModal
      :is-open="isLoginModalOpen"
      @close="isLoginModalOpen = false"
      @success="handleLoginSuccess"
    />

    <!-- 4. 主题切换弹窗 (Theme Switcher Modal) -->
    <ThemeSwitcherModal
      :is-open="isThemeModalOpen"
      @close="isThemeModalOpen = false"
    />

    <!-- 5. 风格内确认弹窗（替代原生 confirm） -->
    <div v-if="confirmState.open" class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-5" :style="{ boxShadow: '6px 6px 0px #121212' }" role="alertdialog" aria-modal="true">
        <h3 class="text-base font-black text-toy-black flex items-center gap-2"><span>⚠️</span>请确认</h3>
        <p class="mt-2 text-xs font-bold text-toy-muted">{{ confirmState.text }}</p>
        <div class="mt-4 flex items-center justify-end gap-2">
          <button @click="confirmNo" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="confirmYes" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-pink text-white text-xs font-black cursor-pointer">确认</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import type { ServerItem, PublicSummary, WGPublicSnapshot, FRPPublicSnapshot } from './types/backend';
import { api, IS_MOCK } from './services/api';
import { formatAgo } from './services/format';

// 基础组件
import ToySidebar from './components/layout/ToySidebar.vue';
import SummaryKpiBar from './components/dashboard/SummaryKpiBar.vue';
import MeshTunnelStatusBar from './components/dashboard/MeshTunnelStatusBar.vue';
import ServerToyCard from './components/dashboard/ServerToyCard.vue';
import ToyTrendChart from './components/charts/ToyTrendChart.vue';
import ThemeSwitcherModal from './components/ThemeSwitcherModal.vue';
import ToyMenuIcons from './components/icons/ToyMenuIcons.vue';
import LoginModal from './components/modals/LoginModal.vue';

// 公开子页面
import ServerDetailView from './components/views/ServerDetailView.vue';
import TunnelView from './components/views/TunnelView.vue';
import MeshPublicView from './components/views/MeshPublicView.vue';

// 管理端子页面
import AdminServersView from './components/views/admin/AdminServersView.vue';
import AdminMeshView from './components/views/admin/AdminMeshView.vue';
import AdminFrpView from './components/views/admin/AdminFrpView.vue';
import AdminSettingsView from './components/views/admin/AdminSettingsView.vue';
import AdminSecurityView from './components/views/admin/AdminSecurityView.vue';
import AdminAuditView from './components/views/admin/AdminAuditView.vue';

// ---------- 常量（对齐 classic monitor store 口径） ----------
const HISTORY_LEN = 40;           // 节点走势滚动窗口
const REFRESH_MS = 10000;         // 保底轮询间隔
const SSE_STALE_MS = 25000;       // 超过此时长没收到任何 SSE 帧 → 判定断流，补全量
const CLOCK_MS = 5000;            // 「更新于 X」计时
const FRP_REFRESH_MS = 60000;     // 穿透摘要是平台侧 3 分钟同步，60s 低频轮询足够
const WG_REFRESH_MS = 60000;      // 组网摘要是巡检回填的静态库表，同低频
// 离线快照（classic doc/15 §离线体验）：断网打开先渲染旧快照，不白屏
const OFFLINE_CACHE_KEY = 'beacontower.snapshot.v1';
const OFFLINE_CACHE_MAX_AGE_S = 7 * 86400;

// 核心状态管理（真实后端：summary 由 servers 归约，SSE 推送增量）
const servers = ref<ServerItem[]>([]);
const summary = ref<PublicSummary>({ total: 0, online: 0, offline: 0, up_bps: 0, down_bps: 0, watts: 0, measured_count: 0, month_kwh: 0, est_cost_month: 0, day_in_total: 0, day_out_total: 0 });
const wgSnapshot = ref<WGPublicSnapshot>({ initialized: false, hubs: [], peers: [], summary: { hub_active: 0, hub_healthy: 0, member_total: 0, member_online: 0, device_total: 0, traffic_today: 0, traffic_month: 0, checked_at: 0 } });
const frpSnapshot = ref<FRPPublicSnapshot>({ platforms: [], summary: { platform_total: 0, tunnel_total: 0, tunnel_online: 0, conns: 0, traffic_today: 0, node_in_use: 0, node_online: 0, updated_at: 0 } });
const globalError = ref('');

// 三态 + 会话失效 + 离线快照标记（classic：loading | ready | error）
const status = ref<'loading' | 'ready' | 'error'>('loading');
const needLogin = ref(false);
const stale = ref(false);
const staleAt = ref(0);
const secondsSinceUpdate = ref(0);
const nowSec = ref(Math.floor(Date.now() / 1000));

// 每节点 40 点走势窗口（classic 由 monitor store 随每帧维护）
const history = ref<Record<number, { cpu: number[]; power: number[] }>>({});

// 详情与弹窗状态
const selectedServer = ref<ServerItem | null>(null);
const isLoginModalOpen = ref(false);
const pendingAdminNav = ref('');

// 管理员鉴权状态
const isAdminLoggedIn = ref(false);

// 导航板块状态
const currentNav = ref('overview');
const isMobileSidebarOpen = ref(false);
const isSidebarCollapsed = ref(false);

const navMetaMap: Record<string, { label: string; iconName: string; desc: string }> = {
  overview: { label: '状态总览', iconName: 'overview', desc: '全网节点网格与实时吞吐走势' },
  'server-detail': { label: '节点详情', iconName: 'overview', desc: '硬件画像与多维历史监控' },
  tunnels: { label: '穿透状态', iconName: 'tunnels', desc: 'Sakura / ChmlFrp / Cloudflare 隧道用量' },
  mesh: { label: '组网状态', iconName: 'mesh', desc: 'WireGuard 星网中心与健康巡检' },
  'admin-servers': { label: '节点管理', iconName: 'admin-servers', desc: '添加/试连/删除节点' },
  'admin-mesh': { label: 'WG 组网管理', iconName: 'admin-mesh', desc: '星型组网拓扑与凭证分发' },
  'admin-frp': { label: '内网穿透托管', iconName: 'admin-frp', desc: '平台绑定与隧道管理' },
  'admin-settings': { label: '采集与展示', iconName: 'admin-settings', desc: '定时周期与电费单价' },
  'admin-security': { label: '安全与账号', iconName: 'admin-security', desc: '管理员密码修改' },
  'admin-audit': { label: '审计日志', iconName: 'admin-audit', desc: '操作历史与审计追踪' },
};

const currentNavMeta = computed(() => {
  if (currentNav.value === 'server-detail' && selectedServer.value) {
    return {
      label: `节点详情 · ${selectedServer.value.name}`,
      iconName: 'overview',
      desc: `${selectedServer.value.profile.os} · ${selectedServer.value.region}`,
    };
  }
  return navMetaMap[currentNav.value] || { label: '信标塔', iconName: 'overview', desc: '监控面板' };
});

// 公开数据三态只作用于公开板块；管理页自管内部加载态
const isPublicNav = computed(() => ['overview', 'tunnels', 'mesh', 'server-detail'].includes(currentNav.value));
const publicLoading = computed(() => isPublicNav.value && status.value === 'loading' && !servers.value.length);
const publicFailed = computed(() => isPublicNav.value && status.value === 'error' && !servers.value.length);

// 电费/今日流量是否真的下发了（关闭「公开页电费」时不能兜成 ¥0.00；
// 无按日流量记录时显示「累积中」而不是 0 B）
const costMeasured = computed(() =>
  servers.value.some((s) => s.status === 'online' && s.power?.est_cost_month != null),
);
const dayMeasured = computed(() => servers.value.some((s) => !!s.traffic_today));

const staleTimeText = computed(() => (staleAt.value ? formatAgo(nowSec.value - staleAt.value) : ''));

function handleNavSelect(key: string) {
  if (key.startsWith('admin-') && !isAdminLoggedIn.value) {
    pendingAdminNav.value = key;
    isLoginModalOpen.value = true;
    return;
  }
  currentNav.value = key;
}

function handleSelectServerDetail(server: ServerItem) {
  selectedServer.value = server;
  currentNav.value = 'server-detail';
}

function openLogin() {
  pendingAdminNav.value = '';
  isLoginModalOpen.value = true;
}

function handleToggleAdmin() {
  if (isAdminLoggedIn.value) {
    openConfirm('退出管理员登录？', async () => {
      try {
        await api.logout();
      } catch {
        /* 会话可能已过期 */
      }
      isAdminLoggedIn.value = false;
      if (currentNav.value.startsWith('admin-')) {
        currentNav.value = 'overview';
      }
    });
  } else {
    openLogin();
  }
}

function handleLoginSuccess() {
  isAdminLoggedIn.value = true;
  isLoginModalOpen.value = false;
  if (pendingAdminNav.value) {
    currentNav.value = pendingAdminNav.value;
    pendingAdminNav.value = '';
  } else {
    currentNav.value = 'admin-servers';
  }
}

async function handleRefreshServers() {
  await fetchAll();
}

// 交互状态
const avatarMode = ref<'bot' | 'os'>('bot');
const isRealtime = ref(true);
const isThemeModalOpen = ref(false);
const searchQuery = ref('');
const currentTab = ref<'all' | 'online' | 'offline'>('all');
const sortBy = ref('default');

function handleUpdateAvatarMode(mode: 'bot' | 'os') {
  avatarMode.value = mode;
  localStorage.setItem('beacontower_avatar_mode', mode);
}

function toggleRealtime() {
  isRealtime.value = !isRealtime.value;
  if (isRealtime.value) {
    api.startStream();
    api.onServers(applyServers);
    void fetchAll();
  } else {
    api.stopStream();
  }
}

// 每节点走势窗口：在线节点逐帧追加 CPU/功耗，离线不追加（classic adaptServer 同口径）
function updateHistories(list: ServerItem[]) {
  const next: Record<number, { cpu: number[]; power: number[] }> = {};
  for (const s of list) {
    const prev = history.value[s.id] || { cpu: [], power: [] };
    const cpu = prev.cpu.slice();
    const power = prev.power.slice();
    if (s.status === 'online') {
      cpu.push(Math.round(s.metrics.cpu_pct));
      while (cpu.length > HISTORY_LEN) cpu.shift();
      if (s.power) {
        power.push(s.power.total_w);
        while (power.length > HISTORY_LEN) power.shift();
      }
    }
    next[s.id] = { cpu, power };
  }
  history.value = next;
}

function applyServers(list: ServerItem[]) {
  servers.value = [...list];
  updateHistories(list);
  summary.value = computeSummary(list);
  secondsSinceUpdate.value = 0;
  nowSec.value = Math.floor(Date.now() / 1000);
}

function computeSummary(list: ServerItem[]): PublicSummary {
  // 与 services/real.ts summarizeServers 同口径（前端归约，SSE 增量直接复用）
  let online = 0, up = 0, down = 0, watts = 0, measured = 0, kwh = 0, cost = 0, dayIn = 0, dayOut = 0;
  for (const s of list) {
    if (s.status === 'online') {
      online++;
      up += s.metrics.net_out_bps;
      down += s.metrics.net_in_bps;
      if (s.power) {
        measured++;
        watts += s.power.total_w;
        kwh += s.power.month_kwh;
        cost += s.power.est_cost_month || 0;
      }
    }
    if (s.traffic_today) {
      dayIn += s.traffic_today.in_total;
      dayOut += s.traffic_today.out_total;
    }
  }
  const r1 = (v: number) => Math.round(v * 10) / 10;
  return { total: list.length, online, offline: list.length - online, up_bps: up, down_bps: down, watts: r1(watts), measured_count: measured, month_kwh: r1(kwh), est_cost_month: r1(cost), day_in_total: dayIn, day_out_total: dayOut };
}

function resetFilters() {
  searchQuery.value = '';
  currentTab.value = 'all';
  sortBy.value = 'default';
}

// 状态筛选（对齐 classic：全部 / 在线 / 离线）
const filterTabs = computed(() => {
  const all = servers.value.length;
  const online = servers.value.filter((s) => s.status === 'online').length;
  return [
    { key: 'all' as const, label: '全部节点', count: all },
    { key: 'online' as const, label: '在线节点', count: online },
    { key: 'offline' as const, label: '离线节点', count: all - online },
  ];
});

const filteredServers = computed(() => {
  let list = [...servers.value];

  if (currentTab.value === 'online') list = list.filter((s) => s.status === 'online');
  else if (currentTab.value === 'offline') list = list.filter((s) => s.status !== 'online');

  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase().trim();
    list = list.filter(s =>
      s.name.toLowerCase().includes(q) ||
      s.region.toLowerCase().includes(q) ||
      s.profile.os.toLowerCase().includes(q) ||
      s.tags.some(t => t.toLowerCase().includes(q))
    );
  }

  if (sortBy.value === 'cpu') {
    list.sort((a, b) => b.metrics.cpu_pct - a.metrics.cpu_pct);
  } else if (sortBy.value === 'mem') {
    list.sort((a, b) => (b.metrics.mem_used / (b.profile.mem_total || 1)) - (a.metrics.mem_used / (a.profile.mem_total || 1)));
  } else if (sortBy.value === 'power') {
    list.sort((a, b) => (b.power?.total_w || 0) - (a.power?.total_w || 0));
  }

  return list;
});

// ---------- 风格内确认弹窗（替代原生 confirm） ----------
const confirmState = ref<{ open: boolean; text: string; action: (() => void | Promise<void>) | null }>({ open: false, text: '', action: null });
function openConfirm(text: string, action: () => void | Promise<void>) {
  confirmState.value = { open: true, text, action };
}
function confirmYes() {
  const a = confirmState.value.action;
  confirmState.value = { open: false, text: '', action: null };
  if (a) void a();
}
function confirmNo() {
  confirmState.value = { open: false, text: '', action: null };
}

// ---------- 离线快照（localStorage，见 doc/15 §离线体验） ----------
function saveSnapshot(list: ServerItem[]) {
  if (!list.length) return;
  try {
    localStorage.setItem(OFFLINE_CACHE_KEY, JSON.stringify({ ts: Math.floor(Date.now() / 1000), servers: list }));
  } catch {
    /* 隐私模式/配额满：快照是增强功能，失败静默 */
  }
}

function loadSnapshot(): boolean {
  let snap: { ts?: number; servers?: ServerItem[] } | null = null;
  try {
    snap = JSON.parse(localStorage.getItem(OFFLINE_CACHE_KEY) || 'null');
  } catch {
    snap = null;
  }
  if (!snap || !Array.isArray(snap.servers) || !snap.servers.length) return false;
  const ageS = Math.floor(Date.now() / 1000) - (snap.ts || 0);
  if (ageS < 0 || ageS > OFFLINE_CACHE_MAX_AGE_S) {
    try {
      localStorage.removeItem(OFFLINE_CACHE_KEY);
    } catch {
      /* 忽略 */
    }
    return false;
  }
  applyServers(snap.servers);
  // 快照垫底：标记 stale（顶栏显示「离线 · 数据时间」），实时数据到达后自动清除
  stale.value = true;
  staleAt.value = snap.ts || 0;
  secondsSinceUpdate.value = Math.min(ageS, 3600);
  status.value = 'ready';
  return true;
}

// ---------- 全局数据装载（servers 首屏 + SSE 增量 + 保底轮询 + 60s WG/FRP） ----------
let pollTimer: ReturnType<typeof setInterval> | null = null;
let clockTimer: ReturnType<typeof setInterval> | null = null;
let wgTimer: ReturnType<typeof setInterval> | null = null;
let frpTimer: ReturnType<typeof setInterval> | null = null;

async function fetchAll() {
  try {
    const list = await api.getServers();
    applyServers(list);
    status.value = 'ready';
    globalError.value = '';
    needLogin.value = false;
    stale.value = false;
    saveSnapshot(list);
  } catch (e) {
    const code = (e as { code?: string } | null)?.code;
    if (code === 'UNAUTHORIZED') needLogin.value = true;
    if (!servers.value.length) {
      // 无快照垫底才打错误页：旧数据好过白屏
      status.value = 'error';
      globalError.value = e instanceof Error ? e.message : '后端连接失败';
    }
  }
}

async function fetchWg() {
  try {
    wgSnapshot.value = await api.getWGPublic();
  } catch {
    /* 私有模式/未初始化时静默，公开态展示 initialized=false */
  }
}

async function fetchFrp() {
  try {
    frpSnapshot.value = await api.getFRPPublic();
  } catch {
    /* 同上 */
  }
}

function reloadAll() {
  if (status.value === 'error') status.value = 'loading';
  void fetchAll();
  void fetchWg();
  void fetchFrp();
}

// 可见性：切后台停 SSE（PWA/标签页语义，防连接堆积），回前台重连并立即刷新
function onVisibilityChange() {
  if (document.hidden) {
    if (isRealtime.value) api.stopStream();
    return;
  }
  if (isRealtime.value && !api.isStreamRunning()) {
    api.startStream();
    api.onServers(applyServers);
  }
  void fetchAll();
}

onMounted(async () => {
  if (IS_MOCK) {
    console.info('[beacontower-toy] 当前运行在 VITE_API_MODE=mock 仿真模式，数据为假数据（仅供 UI 联调）');
  }

  const savedAvatarMode = localStorage.getItem('beacontower_avatar_mode');
  if (savedAvatarMode === 'os' || savedAvatarMode === 'bot') {
    avatarMode.value = savedAvatarMode;
  }

  // 断网也能先渲染旧数据，再被实时数据覆盖
  loadSnapshot();

  // 检查登录状态（真实：/admin/status 会话 cookie）
  try {
    const st = await api.getAdminStatus();
    isAdminLoggedIn.value = st.loggedIn;
  } catch {
    /* 公开态允许 status 失败 */
  }

  api.onServers(applyServers);
  api.startStream();
  await reloadAll();

  // 保底轮询：SSE 活着时退居二线；被反代静默断流（readyState 仍 OPEN、
  // onerror 不触发）时用「多久没收到帧」判活，超时补一次全量
  pollTimer = setInterval(() => {
    if (document.hidden) return;
    const staleStream = !api.isStreamRunning() || Date.now() - api.lastEventAt() > SSE_STALE_MS;
    if (staleStream) void fetchAll();
  }, REFRESH_MS);

  clockTimer = setInterval(() => {
    secondsSinceUpdate.value += CLOCK_MS / 1000;
    nowSec.value = Math.floor(Date.now() / 1000);
  }, CLOCK_MS);

  // WG/FRP 快照低频轮询（对齐 classic monitor store）
  wgTimer = setInterval(() => {
    if (document.hidden) return;
    void fetchWg();
  }, WG_REFRESH_MS);
  frpTimer = setInterval(() => {
    if (document.hidden) return;
    void fetchFrp();
  }, FRP_REFRESH_MS);

  document.addEventListener('visibilitychange', onVisibilityChange);
});

onUnmounted(() => {
  api.stopStream();
  if (pollTimer) clearInterval(pollTimer);
  if (clockTimer) clearInterval(clockTimer);
  if (wgTimer) clearInterval(wgTimer);
  if (frpTimer) clearInterval(frpTimer);
  document.removeEventListener('visibilitychange', onVisibilityChange);
});
</script>
