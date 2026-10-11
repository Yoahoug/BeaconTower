<template>
  <div 
    class="relative inline-flex items-center justify-center rounded-lg border-2 border-toy-black transition-transform duration-150 hover:scale-105 select-none"
    :class="[sizeClasses, colorTheme.bg]"
    :style="{ boxShadow: '2px 2px 0px #121212' }"
  >
    <svg 
      :viewBox="'0 0 32 32'" 
      class="w-full h-full p-1"
      xmlns="http://www.w3.org/2000/svg"
      shape-rendering="crispEdges"
    >
      <!-- 天线 Antenna -->
      <template v-if="antennaType === 0">
        <!-- 居中小天线+圆灯 -->
        <rect x="15" y="2" width="2" height="4" fill="#121212" />
        <rect x="14" y="1" width="4" height="2" :fill="statusColor" />
      </template>
      <template v-else-if="antennaType === 1">
        <!-- 双角天线 -->
        <rect x="7" y="2" width="2" height="4" fill="#121212" />
        <rect x="23" y="2" width="2" height="4" fill="#121212" />
        <rect x="6" y="1" width="4" height="2" :fill="statusColor" />
        <rect x="22" y="1" width="4" height="2" :fill="statusColor" />
      </template>
      <template v-else-if="antennaType === 2">
        <!-- 雷达/闪电天线 -->
        <polygon points="16,1 19,4 16,5 18,7 13,7 15,5 13,4" fill="#FFE047" stroke="#121212" stroke-width="0.5"/>
      </template>
      <template v-else>
        <!-- 螺栓螺帽 -->
        <rect x="4" y="14" width="2" height="4" fill="#121212" />
        <rect x="26" y="14" width="2" height="4" fill="#121212" />
      </template>

      <!-- 机器人头颅轮廓 Robot Head -->
      <rect x="6" y="6" width="20" height="20" rx="3" :fill="colorTheme.face" stroke="#121212" stroke-width="1.5" />

      <!-- 屏幕/面罩内部 Screen Area -->
      <rect x="8" y="9" width="16" height="11" rx="1.5" fill="#121212" />

      <!-- 眼睛 Eyes -->
      <template v-if="eyeType === 0">
        <!-- 经典像素方眼 (绿灯) -->
        <rect x="10" y="12" width="3" height="3" :fill="statusColor" />
        <rect x="19" y="12" width="3" height="3" :fill="statusColor" />
      </template>
      <template v-else-if="eyeType === 1">
        <!-- 横线酷墨镜/护目镜 -->
        <rect x="9" y="13" width="14" height="3" :fill="statusColor" />
      </template>
      <template v-else-if="eyeType === 2">
        <!-- 眨眼 (一圆一横) -->
        <rect x="10" y="12" width="3" height="3" :fill="statusColor" />
        <rect x="18" y="14" width="4" height="1.5" :fill="statusColor" />
      </template>
      <template v-else-if="eyeType === 3">
        <!-- 点阵高光大眼 -->
        <rect x="10" y="11" width="4" height="5" :fill="statusColor" />
        <rect x="18" y="11" width="4" height="5" :fill="statusColor" />
        <rect x="10" y="11" width="1.5" height="1.5" fill="#FFFFFF" />
        <rect x="18" y="11" width="1.5" height="1.5" fill="#FFFFFF" />
      </template>
      <template v-else>
        <!-- 游戏机十字键眼 -->
        <rect x="11" y="12" width="2" height="4" :fill="statusColor" />
        <rect x="10" y="13" width="4" height="2" :fill="statusColor" />
        <rect x="19" y="12" width="2" height="4" :fill="statusColor" />
        <rect x="18" y="13" width="4" height="2" :fill="statusColor" />
      </template>

      <!-- 嘴巴 / 散热腮裂 Mouth / Grille -->
      <template v-if="mouthType === 0">
        <!-- 三点点阵笑脸 -->
        <rect x="11" y="22" width="2" height="1.5" fill="#121212" />
        <rect x="15" y="22" width="2" height="1.5" fill="#121212" />
        <rect x="19" y="22" width="2" height="1.5" fill="#121212" />
      </template>
      <template v-else-if="mouthType === 1">
        <!-- 经典螺纹波形口 -->
        <path d="M 11 22 L 13 24 L 15 22 L 17 24 L 19 22" stroke="#121212" stroke-width="1.2" fill="none" stroke-linecap="round"/>
      </template>
      <template v-else>
        <!-- 紧凑单条开口 -->
        <rect x="12" y="22" width="8" height="1.5" rx="0.5" fill="#121212" />
      </template>
    </svg>

    <!-- 在线/离线微型状态小指示灯 -->
    <span 
      class="absolute -top-1 -right-1 w-3 h-3 rounded-full border border-toy-black flex items-center justify-center"
      :class="isOnline ? 'bg-toy-mint animate-pulse-slow' : 'bg-toy-muted'"
    >
      <span class="w-1 h-1 rounded-full bg-white"></span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    seed: string | number;
    isOnline?: boolean;
    size?: 'sm' | 'md' | 'lg';
  }>(),
  {
    isOnline: true,
    size: 'md',
  }
);

// 尺寸样式
const sizeClasses = computed(() => {
  switch (props.size) {
    case 'sm': return 'w-8 h-8';
    case 'lg': return 'w-14 h-14';
    default: return 'w-11 h-11';
  }
});

// 字符串哈希算法
function hashString(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    hash = (hash << 5) - hash + str.charCodeAt(i);
    hash |= 0;
  }
  return Math.abs(hash);
}

const hash = computed(() => hashString(String(props.seed)));

// 玩具色系组合
const THEMES = [
  { bg: 'bg-[#FFE047]', face: '#FFF3A3' }, // 柠檬黄
  { bg: 'bg-[#FF5E85]', face: '#FFE3EB' }, // 蜜桃粉
  { bg: 'bg-[#00E599]', face: '#D6FBEF' }, // 薄荷绿
  { bg: 'bg-[#38BDF8]', face: '#E0F7FE' }, // 电子天蓝
  { bg: 'bg-[#FB923C]', face: '#FFEDD5' }, // 活力橙
  { bg: 'bg-[#A78BFA]', face: '#EDE9FE' }, // 薰衣草紫
];

const colorTheme = computed(() => {
  const idx = hash.value % THEMES.length;
  return THEMES[idx];
});

const antennaType = computed(() => (hash.value >> 2) % 4);
const eyeType = computed(() => (hash.value >> 4) % 5);
const mouthType = computed(() => (hash.value >> 6) % 3);

const statusColor = computed(() => {
  if (!props.isOnline) return '#71717A'; // 离线灰色
  const eyeColors = ['#00E599', '#38BDF8', '#FFE047', '#FF5E85'];
  return eyeColors[(hash.value >> 8) % eyeColors.length];
});
</script>
