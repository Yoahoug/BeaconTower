<template>
  <div>
    <!-- 移动端遮罩 (Backdrop) -->
    <div 
      v-if="isOpen" 
      class="fixed inset-0 z-40 bg-toy-black/50 backdrop-blur-xs lg:hidden transition-opacity"
      @click="$emit('close')"
    ></div>

    <!-- 侧边栏主体 (Sidebar Aside) -->
    <aside 
      class="fixed top-0 bottom-0 left-0 z-40 bg-white border-r-2.5 border-toy-black flex flex-col transition-all duration-200 select-none"
      :class="[
        isOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0',
        collapsed ? 'lg:w-20' : 'w-64 lg:w-64',
      ]"
      :style="{ boxShadow: '4px 0px 0px #121212' }"
    >
      <!-- 1. 顶栏：Logo 与折叠切换 -->
      <div class="p-3.5 border-b-2.5 border-toy-black flex items-center justify-between gap-2">
        <!-- 未折叠状态：展示完整 Logo -->
        <div v-if="!collapsed" class="flex items-center gap-2.5 min-w-0">
          <div 
            class="w-9 h-9 rounded-xl bg-toy-yellow border-2 border-toy-black flex items-center justify-center shrink-0"
            :style="{ boxShadow: '2px 2px 0px #121212' }"
          >
            <!-- 矢量微型灯塔 -->
            <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none">
              <rect x="5" y="19" width="14" height="3" rx="1" fill="#121212"/>
              <polygon points="7,19 9,8 15,8 17,19" fill="#FFFFFF" stroke="#121212" stroke-width="1.8"/>
              <rect x="8" y="5" width="8" height="3" rx="0.5" fill="#FFE047" stroke="#121212" stroke-width="1.5"/>
              <path d="M9 5 C9 2 15 2 15 5 Z" fill="#FF5E85" stroke="#121212" stroke-width="1.5"/>
              <line x1="12" y1="2" x2="12" y2="0" stroke="#121212" stroke-width="1.8" stroke-linecap="round"/>
            </svg>
          </div>
          <div class="min-w-0">
            <h1 class="text-base font-black text-toy-black truncate leading-tight">BeaconTower</h1>
            <p class="text-[10px] font-bold text-toy-muted tracking-wider">信标塔监控</p>
          </div>
        </div>

        <!-- 折叠状态：顶栏居中显示可点击展开的微型 Logo 按钮 -->
        <div v-else class="w-full flex justify-center">
          <button 
            @click="toggleCollapse"
            class="w-10 h-10 rounded-xl bg-toy-yellow border-2 border-toy-black flex items-center justify-center transition-transform hover:scale-105 active:scale-95 cursor-pointer"
            :style="{ boxShadow: '2px 2px 0px #121212' }"
            title="点击展开侧边栏"
          >
            <svg class="w-6 h-6" viewBox="0 0 24 24" fill="none">
              <rect x="5" y="19" width="14" height="3" rx="1" fill="#121212"/>
              <polygon points="7,19 9,8 15,8 17,19" fill="#FFFFFF" stroke="#121212" stroke-width="1.8"/>
              <rect x="8" y="5" width="8" height="3" rx="0.5" fill="#FFE047" stroke="#121212" stroke-width="1.5"/>
              <path d="M9 5 C9 2 15 2 15 5 Z" fill="#FF5E85" stroke="#121212" stroke-width="1.5"/>
            </svg>
          </button>
        </div>

        <!-- 桌面端折叠切换按钮 -->
        <button 
          v-if="!collapsed"
          @click="toggleCollapse"
          class="hidden lg:flex w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-yellow items-center justify-center text-xs font-black transition-colors cursor-pointer"
          title="收缩侧边栏"
        >
          ◀
        </button>
      </div>

      <!-- 2. 导航列表区域 -->
      <nav class="flex-1 overflow-y-auto p-3 space-y-4 scrollbar-none">
        <!-- 分组 1: 监控板块 (公开访问) -->
        <div>
          <div 
            v-if="!collapsed" 
            class="px-2 py-1 text-[11px] font-black uppercase tracking-wider text-toy-muted flex items-center justify-between"
          >
            <span>监控板块</span>
            <span class="text-[9px] font-bold text-toy-black bg-toy-bg px-1 rounded border border-toy-black">公开</span>
          </div>
          <div class="space-y-1 mt-1">
            <button 
              v-for="item in monitorNav" 
              :key="item.key"
              @click="handleSelect(item.key)"
              class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded-xl text-xs font-extrabold transition-all border-2 group cursor-pointer"
              :class="activeKey === item.key 
                ? 'bg-toy-yellow text-toy-black border-toy-black shadow-toy-sm translate-x-1' 
                : 'border-transparent hover:bg-toy-bg hover:border-toy-black text-toy-black'"
              :title="item.label"
            >
              <!-- 纯 SVG 玩具图标独立底座 (带粗黑边与独立阴影，与背景色分明) -->
              <div 
                class="w-7 h-7 rounded-lg border-2 border-toy-black flex items-center justify-center shrink-0 transition-transform bg-white"
                :style="{ boxShadow: '2px 2px 0px #121212' }"
                :class="activeKey === item.key ? 'scale-105' : 'group-hover:scale-105'"
              >
                <ToyMenuIcons :name="item.icon" class="w-4 h-4" />
              </div>

              <span v-if="!collapsed" class="truncate font-extrabold">{{ item.label }}</span>
              <!-- 在线状态胶囊 -->
              <span 
                v-if="!collapsed && item.badge"
                class="ml-auto px-1.5 py-0.2 text-[9px] font-extrabold rounded border border-toy-black"
                :class="item.badgeClass || 'bg-toy-mint text-toy-black'"
              >
                {{ item.badge }}
              </span>
            </button>
          </div>
        </div>

        <!-- 分组 2: 管理面板 (严格对齐原版：仅登录后展示) -->
        <div v-if="isAdminLoggedIn">
          <div 
            v-if="!collapsed" 
            class="px-2 py-1 text-[11px] font-black uppercase tracking-wider text-toy-muted flex items-center justify-between"
          >
            <span>管理面板</span>
            <span class="text-[9px] font-bold bg-toy-yellow text-toy-black px-1 rounded border border-toy-black">已鉴权</span>
          </div>
          <div class="space-y-1 mt-1">
            <button 
              v-for="item in adminNav" 
              :key="item.key"
              @click="handleSelect(item.key)"
              class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded-xl text-xs font-extrabold transition-all border-2 group cursor-pointer"
              :class="activeKey === item.key 
                ? 'bg-toy-yellow text-toy-black border-toy-black shadow-toy-sm translate-x-1' 
                : 'border-transparent hover:bg-toy-bg hover:border-toy-black text-toy-black'"
              :title="item.label"
            >
              <!-- 纯 SVG 玩具图标独立底座 (带粗黑边与独立阴影) -->
              <div 
                class="w-7 h-7 rounded-lg border-2 border-toy-black flex items-center justify-center shrink-0 transition-transform bg-white"
                :style="{ boxShadow: '2px 2px 0px #121212' }"
                :class="activeKey === item.key ? 'scale-105' : 'group-hover:scale-105'"
              >
                <ToyMenuIcons :name="item.icon" class="w-4 h-4" />
              </div>

              <span v-if="!collapsed" class="truncate font-extrabold">{{ item.label }}</span>
            </button>

            <!-- 退出登录按钮 (对齐原版) -->
            <button 
              @click="toggleAdminLogin"
              class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded-xl text-xs font-bold transition-all border-2 border-transparent hover:bg-red-50 hover:border-toy-black text-red-600 mt-2 cursor-pointer group"
              title="退出管理员身份"
            >
              <div 
                class="w-7 h-7 rounded-lg border-2 border-toy-black flex items-center justify-center shrink-0 bg-red-100"
                :style="{ boxShadow: '2px 2px 0px #121212' }"
              >
                <ToyMenuIcons name="logout" class="w-4 h-4" />
              </div>
              <span v-if="!collapsed" class="truncate">退出管理员</span>
            </button>
          </div>
        </div>

        <!-- 游客态：对齐原版设计，低调管理入口 -->
        <div v-if="!isAdminLoggedIn && !collapsed" class="px-2 pt-2">
          <button 
            @click="toggleAdminLogin"
            class="w-full py-1.5 px-3 rounded-lg text-xs font-bold text-toy-muted hover:text-toy-black hover:bg-toy-bg flex items-center justify-between transition-colors border border-dashed border-toy-gray cursor-pointer"
            title="管理员登录"
          >
            <span class="flex items-center gap-2">
              <ToyMenuIcons name="security" class="w-3.5 h-3.5" />
              <span>管理员登录</span>
            </span>
            <span class="text-[10px] text-toy-muted">➔</span>
          </button>
        </div>
      </nav>

      <!-- 3. 左下角：精简微型控制栏 + 全网状态 + 版本号 (对齐原版风格) -->
      <div class="p-3 border-t-2.5 border-toy-black bg-white space-y-2.5">
        <!-- 未折叠状态 -->
        <template v-if="!collapsed">
          <!-- 微型设置条：头像模式 + 主题切换 -->
          <div class="flex items-center justify-between p-1 bg-toy-bg rounded-lg border border-toy-black">
            <!-- 1. 头像模式快速切换 (小胶囊) -->
            <button 
              @click="$emit('update-avatar-mode', avatarMode === 'bot' ? 'os' : 'bot')"
              class="px-2 py-1 rounded text-[11px] font-black flex items-center gap-1.5 transition-colors hover:bg-white cursor-pointer"
              :title="avatarMode === 'bot' ? '当前：机器人头像 (点击切换为系统标)' : '当前：系统发行版徽标 (点击切换为机器人)'"
            >
              <span>{{ avatarMode === 'bot' ? '🤖' : '🐧' }}</span>
              <span class="text-[10px] text-toy-black">{{ avatarMode === 'bot' ? '机器人' : '系统标' }}</span>
            </button>

            <span class="text-toy-gray">|</span>

            <!-- 2. 主题切换 (小调色板) -->
            <button 
              @click="$emit('open-theme-modal')"
              class="px-2 py-1 rounded text-[11px] font-black flex items-center gap-1.5 transition-colors hover:bg-white text-toy-black cursor-pointer"
              title="切换视觉主题风格"
            >
              <span>🎨</span>
              <span class="text-[10px]">主题</span>
            </button>
          </div>

          <!-- 全网在线与功率实时微标 -->
          <div class="flex items-center justify-between text-[11px] font-mono font-bold text-toy-black bg-toy-bg/60 px-2 py-1.5 rounded-lg border border-toy-gray">
            <span class="flex items-center gap-1.5">
              <span class="w-2 h-2 rounded-full bg-toy-mint animate-pulse"></span>
              <span>节点 {{ onlineCount }}/{{ totalCount }}</span>
            </span>
            <span class="text-amber-800">
              ⚡ {{ totalWatts.toFixed(1) }} W
            </span>
          </div>

          <!-- 正规产品版本号 (对齐原版 v2.4 · SKY BEACON) -->
          <div class="flex items-center justify-between text-[10px] font-bold text-toy-muted px-1">
            <span>v2.8.4 · TOY BEACON</span>
            <span class="inline-flex items-center gap-1 text-[9px] text-toy-mint">
              <span class="w-1.5 h-1.5 rounded-full bg-toy-mint"></span> 运行中
            </span>
          </div>
        </template>

        <!-- 折叠状态：紧凑图标控制 -->
        <template v-else>
          <div class="flex flex-col items-center gap-2">
            <!-- 头像切换微标 (带立体阴影) -->
            <button 
              @click="$emit('update-avatar-mode', avatarMode === 'bot' ? 'os' : 'bot')"
              class="w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-yellow flex items-center justify-center text-xs font-bold cursor-pointer"
              :style="{ boxShadow: '2px 2px 0px #121212' }"
              :title="avatarMode === 'bot' ? '当前：机器人头像 (点击切为系统标)' : '当前：系统徽标 (点击切为机器人)'"
            >
              {{ avatarMode === 'bot' ? '🤖' : '🐧' }}
            </button>

            <!-- 主题切换微标 (带立体阴影) -->
            <button 
              @click="$emit('open-theme-modal')"
              class="w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-yellow flex items-center justify-center text-xs font-bold cursor-pointer"
              :style="{ boxShadow: '2px 2px 0px #121212' }"
              title="切换主题"
            >
              🎨
            </button>

            <!-- 展开侧栏箭头按钮 (带立体阴影) -->
            <button 
              @click="toggleCollapse"
              class="w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-gray hover:bg-toy-yellow flex items-center justify-center text-xs font-black cursor-pointer mt-1"
              :style="{ boxShadow: '2px 2px 0px #121212' }"
              title="展开侧栏"
            >
              ▶
            </button>
          </div>
        </template>
      </div>
    </aside>
  </div>
</template>

<script setup lang="ts">
import ToyMenuIcons from '../icons/ToyMenuIcons.vue';

const props = defineProps<{
  isOpen: boolean;
  activeKey: string;
  onlineCount: number;
  totalCount: number;
  totalWatts: number;
  avatarMode: 'bot' | 'os';
  isAdminLoggedIn: boolean;
  collapsed: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'select', key: string): void;
  (e: 'update-avatar-mode', mode: 'bot' | 'os'): void;
  (e: 'open-theme-modal'): void;
  (e: 'toggle-admin'): void;
  (e: 'update:collapsed', val: boolean): void;
}>();

// 对应原版前端的监控板块 (公开访问)
interface NavItem {
  key: string;
  label: string;
  icon: string;
  badge?: string;
  badgeClass?: string;
}

const monitorNav: NavItem[] = [
  { key: 'overview', label: '状态总览', icon: 'overview', badge: '实时', badgeClass: 'bg-toy-mint text-toy-black' },
  { key: 'tunnels', label: '穿透状态', icon: 'tunnels', badge: '3平台', badgeClass: 'bg-toy-yellow text-toy-black' },
  { key: 'mesh', label: '组网状态', icon: 'mesh', badge: 'Mesh', badgeClass: 'bg-toy-cyan text-toy-black' },
];

// 对应原版前端的管理面板板块 (登录后展示)
const adminNav: NavItem[] = [
  { key: 'admin-servers', label: '节点管理', icon: 'admin-servers' },
  { key: 'admin-mesh', label: 'WG 组网', icon: 'admin-mesh' },
  { key: 'admin-frp', label: '内网穿透', icon: 'admin-frp' },
  { key: 'admin-settings', label: '采集与展示', icon: 'admin-settings' },
  { key: 'admin-security', label: '安全与账号', icon: 'admin-security' },
  { key: 'admin-audit', label: '审计日志', icon: 'admin-audit' },
];

function handleSelect(key: string) {
  emit('select', key);
  emit('close');
}

function toggleAdminLogin() {
  emit('toggle-admin');
}

function toggleCollapse() {
  emit('update:collapsed', !props.collapsed);
}
</script>
