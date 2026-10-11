<template>
  <div class="flex items-center gap-3 select-none">
    <div 
      class="w-10 h-10 rounded-xl bg-toy-yellow border-2.5 border-toy-black flex items-center justify-center relative overflow-hidden transition-transform hover:scale-105 active:scale-95"
      :style="{ boxShadow: '3px 3px 0px #121212' }"
    >
      <svg class="w-7 h-7" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg">
        <!-- 塔底 -->
        <rect x="5" y="25" width="22" height="4" rx="1.5" fill="#121212" />
        <!-- 塔身 -->
        <polygon points="9,25 12,12 20,12 23,25" fill="#FFFFFF" stroke="#121212" stroke-width="1.8" stroke-linejoin="round"/>
        <!-- 塔身红杠 -->
        <polygon points="10.5,19 11.5,15 20.5,15 21.5,19" fill="#FF5E85" stroke="#121212" stroke-width="1.2"/>
        <!-- 观景平台 -->
        <rect x="10" y="10" width="12" height="2.5" rx="1" fill="#FFE047" stroke="#121212" stroke-width="1.5"/>
        <!-- 灯室 -->
        <rect x="12" y="5" width="8" height="5" rx="1" fill="#FFF3A3" stroke="#121212" stroke-width="1.5"/>
        <!-- 发光圆球 -->
        <circle cx="16" cy="7.5" r="2" fill="#FFE047" stroke="#121212" stroke-width="1"/>
        <!-- 塔顶半球 -->
        <path d="M 12 5 C 12 1.5 20 1.5 20 5 Z" fill="#FF5E85" stroke="#121212" stroke-width="1.5"/>
        <!-- 塔尖小天线 -->
        <line x1="16" y1="1.5" x2="16" y2="0" stroke="#121212" stroke-width="1.8" stroke-linecap="round"/>
      </svg>
      <!-- 动态闪烁光斑 -->
      <span class="absolute top-1 right-1 w-2 h-2 rounded-full bg-toy-mint animate-pulse-slow"></span>
    </div>
    
    <div>
      <div class="flex items-center gap-1.5">
        <h1 class="text-xl font-extrabold tracking-tight text-toy-black leading-none">BeaconTower</h1>
        <span class="px-1.5 py-0.5 text-[10px] font-black uppercase rounded bg-toy-yellow text-toy-black border border-toy-black">Toy</span>
      </div>
      <p class="text-[11px] font-bold text-toy-muted tracking-wider leading-tight mt-0.5">信标塔 · 极简监控面板</p>
    </div>
  </div>
</template>
