<template>
  <div 
    class="relative inline-flex items-center justify-center rounded-lg border-2 border-toy-black transition-transform duration-150 hover:scale-105 select-none"
    :class="[sizeClasses, badgeInfo.bgColor]"
    :style="{ boxShadow: '2px 2px 0px #121212' }"
    :title="osName"
  >
    <svg :viewBox="'0 0 32 32'" class="w-full h-full p-1.5" xmlns="http://www.w3.org/2000/svg">
      <!-- Debian: 红色经典漩涡 -->
      <template v-if="badgeInfo.key === 'debian'">
        <circle cx="16" cy="16" r="12" fill="#D70A53" stroke="#121212" stroke-width="1.5"/>
        <path d="M16,7 C11,7 8,11 8,15 C8,20 12,24 17,24 C21,24 23,21 23,18 C23,14 19,13 17,15 C15,17 17,19 18,18" 
          fill="none" stroke="#FFFFFF" stroke-width="2.5" stroke-linecap="round"/>
      </template>

      <!-- Ubuntu: 橙红圆环与三点 -->
      <template v-else-if="badgeInfo.key === 'ubuntu'">
        <circle cx="16" cy="16" r="12" fill="#E95420" stroke="#121212" stroke-width="1.5"/>
        <circle cx="16" cy="16" r="6" fill="#FFFFFF" stroke="#121212" stroke-width="1.2"/>
        <circle cx="16" cy="7" r="2.2" fill="#FFFFFF" stroke="#121212" stroke-width="1"/>
        <circle cx="8" cy="21" r="2.2" fill="#FFFFFF" stroke="#121212" stroke-width="1"/>
        <circle cx="24" cy="21" r="2.2" fill="#FFFFFF" stroke="#121212" stroke-width="1"/>
      </template>

      <!-- Arch Linux: 青蓝山形折线 -->
      <template v-else-if="badgeInfo.key === 'arch'">
        <circle cx="16" cy="16" r="12" fill="#1793D1" stroke="#121212" stroke-width="1.5"/>
        <path d="M16,7 L23,23 L18,22 L16,17 L14,22 L9,23 Z" fill="#FFFFFF" stroke="#121212" stroke-width="1.5"/>
      </template>

      <!-- Alpine: 冰蓝三角雪山 -->
      <template v-else-if="badgeInfo.key === 'alpine'">
        <circle cx="16" cy="16" r="12" fill="#0D597F" stroke="#121212" stroke-width="1.5"/>
        <path d="M16,8 L7,24 L25,24 Z" fill="#38BDF8" stroke="#121212" stroke-width="1.5"/>
        <path d="M16,8 L12,16 L17,17 L15,20 L21,20 L16,8 Z" fill="#FFFFFF"/>
      </template>

      <!-- CentOS: 极光魔方十字 -->
      <template v-else-if="badgeInfo.key === 'centos'">
        <circle cx="16" cy="16" r="12" fill="#262577" stroke="#121212" stroke-width="1.5"/>
        <rect x="13.5" y="6" width="5" height="20" fill="#FFA500" stroke="#121212" stroke-width="1"/>
        <rect x="6" y="13.5" width="20" height="5" fill="#93227F" stroke="#121212" stroke-width="1"/>
        <circle cx="16" cy="16" r="3" fill="#22C55E"/>
      </template>

      <!-- Docker / 容器环境 -->
      <template v-else-if="badgeInfo.key === 'docker'">
        <circle cx="16" cy="16" r="12" fill="#2496ED" stroke="#121212" stroke-width="1.5"/>
        <!-- 小鲸鱼背部积木 -->
        <rect x="9" y="11" width="3" height="3" fill="#FFFFFF" stroke="#121212" stroke-width="0.8"/>
        <rect x="13" y="11" width="3" height="3" fill="#FFFFFF" stroke="#121212" stroke-width="0.8"/>
        <rect x="17" y="11" width="3" height="3" fill="#FFFFFF" stroke="#121212" stroke-width="0.8"/>
        <rect x="13" y="7" width="3" height="3" fill="#FFFFFF" stroke="#121212" stroke-width="0.8"/>
        <!-- 鲸鱼身子 -->
        <path d="M6,16 C8,15 22,15 25,18 C26,21 21,23 15,23 C9,23 6,19 6,16 Z" fill="#FFFFFF" stroke="#121212" stroke-width="1.2"/>
      </template>

      <!-- 通用 Linux / 物理机 -->
      <template v-else>
        <circle cx="16" cy="16" r="12" fill="#FFE047" stroke="#121212" stroke-width="1.5"/>
        <rect x="11" y="9" width="10" height="14" rx="4" fill="#121212"/>
        <circle cx="14" cy="13" r="1.5" fill="#FFFFFF"/>
        <circle cx="18" cy="13" r="1.5" fill="#FFFFFF"/>
        <polygon points="16,15 14,18 18,18" fill="#FFA500"/>
        <ellipse cx="16" cy="23" rx="5" ry="2" fill="#FFA500" stroke="#121212" stroke-width="1"/>
      </template>
    </svg>

    <!-- 在线指示灯 -->
    <span 
      v-if="isOnline"
      class="absolute -top-1 -right-1 w-3 h-3 rounded-full border border-toy-black bg-toy-mint animate-pulse-slow flex items-center justify-center"
    >
      <span class="w-1 h-1 rounded-full bg-white"></span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    osName: string;
    isOnline?: boolean;
    size?: 'sm' | 'md' | 'lg';
  }>(),
  {
    isOnline: true,
    size: 'md',
  }
);

const sizeClasses = computed(() => {
  switch (props.size) {
    case 'sm': return 'w-8 h-8';
    case 'lg': return 'w-14 h-14';
    default: return 'w-11 h-11';
  }
});

const badgeInfo = computed(() => {
  const name = (props.osName || '').toLowerCase();
  if (name.includes('debian')) {
    return { key: 'debian', bgColor: 'bg-red-50' };
  } else if (name.includes('ubuntu')) {
    return { key: 'ubuntu', bgColor: 'bg-orange-50' };
  } else if (name.includes('arch')) {
    return { key: 'arch', bgColor: 'bg-sky-50' };
  } else if (name.includes('alpine')) {
    return { key: 'alpine', bgColor: 'bg-blue-50' };
  } else if (name.includes('centos') || name.includes('redhat') || name.includes('rhel')) {
    return { key: 'centos', bgColor: 'bg-purple-50' };
  } else if (name.includes('docker')) {
    return { key: 'docker', bgColor: 'bg-cyan-50' };
  }
  return { key: 'linux', bgColor: 'bg-yellow-50' };
});
</script>
