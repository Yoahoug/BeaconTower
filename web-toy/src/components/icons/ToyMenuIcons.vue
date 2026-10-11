<template>
  <svg 
    :viewBox="'0 0 24 24'" 
    class="w-5 h-5 shrink-0"
    xmlns="http://www.w3.org/2000/svg"
    fill="none"
  >
    <!-- 1. 状态总览 (Overview / Dashboard: 粗黑四格积木仪表盘) -->
    <template v-if="name === 'overview' || name === 'dashboard'">
      <rect x="3" y="3" width="8" height="8" rx="2" fill="#FFE047" stroke="#121212" stroke-width="2.2" stroke-linejoin="round"/>
      <rect x="13" y="3" width="8" height="5" rx="1.5" fill="#FFFFFF" stroke="#121212" stroke-width="2.2" stroke-linejoin="round"/>
      <rect x="3" y="13" width="8" height="8" rx="2" fill="#FFFFFF" stroke="#121212" stroke-width="2.2" stroke-linejoin="round"/>
      <rect x="13" y="10" width="8" height="11" rx="2" fill="#38BDF8" stroke="#121212" stroke-width="2.2" stroke-linejoin="round"/>
    </template>

    <!-- 2. 穿透状态 (Tunnels: 双向管道 / 闪电双接头) -->
    <template v-else-if="name === 'tunnels' || name === 'tunnel'">
      <path d="M4 14H10L8 20L15 10H9L11 4L4 14Z" fill="#FFE047" stroke="#121212" stroke-width="2.2" stroke-linejoin="round"/>
      <circle cx="18" cy="6" r="2" fill="#00E599" stroke="#121212" stroke-width="1.8"/>
      <circle cx="19" cy="18" r="2" fill="#FF5E85" stroke="#121212" stroke-width="1.8"/>
    </template>

    <!-- 3. 组网状态 (Mesh / WireGuard 星型节点网络) -->
    <template v-else-if="name === 'mesh'">
      <!-- 中心节点 -->
      <circle cx="12" cy="12" r="3.5" fill="#FFE047" stroke="#121212" stroke-width="2.2"/>
      <!-- 外围三节点 -->
      <circle cx="12" cy="4" r="2" fill="#FFFFFF" stroke="#121212" stroke-width="2"/>
      <circle cx="4" cy="18" r="2" fill="#FFFFFF" stroke="#121212" stroke-width="2"/>
      <circle cx="20" cy="18" r="2" fill="#FFFFFF" stroke="#121212" stroke-width="2"/>
      <!-- 连接线 -->
      <line x1="12" y1="6" x2="12" y2="8.5" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
      <line x1="5.8" y1="16.5" x2="9.5" y2="13.8" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
      <line x1="18.2" y1="16.5" x2="14.5" y2="13.8" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
    </template>

    <!-- 4. 节点管理 (Admin Servers: 双层服务器机架积木) -->
    <template v-else-if="name === 'servers' || name === 'admin-servers'">
      <rect x="3" y="4" width="18" height="6" rx="2" fill="#FFFFFF" stroke="#121212" stroke-width="2.2"/>
      <circle cx="6.5" cy="7" r="1" fill="#00E599"/>
      <line x1="10" y1="7" x2="17" y2="7" stroke="#121212" stroke-width="1.8" stroke-linecap="round"/>
      
      <rect x="3" y="14" width="18" height="6" rx="2" fill="#FFE047" stroke="#121212" stroke-width="2.2"/>
      <circle cx="6.5" cy="17" r="1" fill="#00E599"/>
      <line x1="10" y1="17" x2="17" y2="17" stroke="#121212" stroke-width="1.8" stroke-linecap="round"/>
    </template>

    <!-- 5. WG 组网管理 (Admin Mesh: 盾牌 + 星线) -->
    <template v-else-if="name === 'admin-mesh'">
      <path d="M12 3L4 6V12C4 17 8 20 12 21C16 20 20 17 20 12V6L12 3Z" fill="#FFFFFF" stroke="#121212" stroke-width="2.2" stroke-linejoin="round"/>
      <path d="M12 7V16M8 11.5L16 11.5" stroke="#121212" stroke-width="2.2" stroke-linecap="round"/>
      <circle cx="12" cy="11.5" r="2" fill="#FFE047" stroke="#121212" stroke-width="1.8"/>
    </template>

    <!-- 6. 内网穿透托管 (Admin FRP: 容器方块 + 双向端口) -->
    <template v-else-if="name === 'admin-frp'">
      <rect x="4" y="5" width="16" height="14" rx="2.5" fill="#FFFFFF" stroke="#121212" stroke-width="2.2"/>
      <rect x="7" y="8" width="4" height="4" fill="#38BDF8" stroke="#121212" stroke-width="1.5"/>
      <rect x="13" y="8" width="4" height="4" fill="#FFE047" stroke="#121212" stroke-width="1.5"/>
      <path d="M8 15H16M16 15L14 13M16 15L14 17" stroke="#121212" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
    </template>

    <!-- 7. 采集与展示 (Settings: 调音台推子) -->
    <template v-else-if="name === 'settings' || name === 'admin-settings'">
      <line x1="6" y1="4" x2="6" y2="20" stroke="#121212" stroke-width="2.2" stroke-linecap="round"/>
      <line x1="18" y1="4" x2="18" y2="20" stroke="#121212" stroke-width="2.2" stroke-linecap="round"/>
      <line x1="12" y1="4" x2="12" y2="20" stroke="#121212" stroke-width="2.2" stroke-linecap="round"/>
      <rect x="4" y="7" width="4" height="5" rx="1.5" fill="#FFE047" stroke="#121212" stroke-width="2"/>
      <rect x="10" y="13" width="4" height="5" rx="1.5" fill="#FF5E85" stroke="#121212" stroke-width="2"/>
      <rect x="16" y="9" width="4" height="5" rx="1.5" fill="#00E599" stroke="#121212" stroke-width="2"/>
    </template>

    <!-- 8. 安全与账号 (Security: 粗黑实体挂锁) -->
    <template v-else-if="name === 'security' || name === 'admin-security'">
      <rect x="5" y="10" width="14" height="11" rx="2.5" fill="#FFE047" stroke="#121212" stroke-width="2.2"/>
      <path d="M8 10V6.5C8 4.5 9.8 3 12 3C14.2 3 16 4.5 16 6.5V10" stroke="#121212" stroke-width="2.2" stroke-linecap="round"/>
      <circle cx="12" cy="15" r="1.5" fill="#121212"/>
      <line x1="12" y1="16.5" x2="12" y2="18.5" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
    </template>

    <!-- 9. 审计日志 (Audit: 粗黑卷轴清单) -->
    <template v-else-if="name === 'audit' || name === 'admin-audit'">
      <rect x="4" y="3" width="16" height="18" rx="2" fill="#FFFFFF" stroke="#121212" stroke-width="2.2"/>
      <line x1="8" y1="8" x2="16" y2="8" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
      <line x1="8" y1="12" x2="14" y2="12" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
      <line x1="8" y1="16" x2="11" y2="16" stroke="#121212" stroke-width="2" stroke-linecap="round"/>
      <circle cx="16" cy="16" r="1.5" fill="#FF5E85"/>
    </template>

    <!-- 10. 退出登录 (Logout) -->
    <template v-else-if="name === 'logout'">
      <path d="M9 21H5C3.9 21 3 20.1 3 19V5C3 3.9 3.9 3 5 3H9" stroke="#121212" stroke-width="2.2" stroke-linecap="round"/>
      <path d="M16 17L21 12L16 7M21 12H9" stroke="#121212" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
    </template>

    <!-- 11. 登录 (Login) -->
    <template v-else-if="name === 'login'">
      <rect x="3" y="11" width="18" height="10" rx="2" fill="#FFFFFF" stroke="#121212" stroke-width="2.2"/>
      <path d="M7 11V7C7 4.5 9 2.5 12 2.5C15 2.5 17 4.5 17 7V11" stroke="#121212" stroke-width="2.2"/>
      <circle cx="12" cy="16" r="1.5" fill="#FFE047"/>
    </template>

    <!-- 兜底图标 -->
    <template v-else>
      <circle cx="12" cy="12" r="8" fill="#FFE047" stroke="#121212" stroke-width="2.2"/>
    </template>
  </svg>
</template>

<script setup lang="ts">
defineProps<{
  name: string;
}>();
</script>
