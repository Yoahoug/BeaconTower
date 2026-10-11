<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm">
    <div
      class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 relative overflow-hidden"
      :style="{ boxShadow: '6px 6px 0px #121212' }"
      role="dialog"
      aria-modal="true"
      aria-label="视觉风格与主题中心"
    >
      <!-- 弹窗顶栏 -->
      <div class="flex items-center justify-between pb-4 border-b-2.5 border-toy-black">
        <div class="flex items-center gap-2">
          <span class="text-2xl">🎨</span>
          <div>
            <h3 class="text-lg font-black text-toy-black">视觉风格与主题中心</h3>
            <p class="text-xs font-bold text-toy-muted">自由切换您喜爱的操作界面风格</p>
          </div>
        </div>
        <button
          @click="$emit('close')"
          class="w-8 h-8 rounded-lg border-2 border-toy-black bg-toy-gray/60 hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-sm transition-colors"
          aria-label="关闭"
        >
          ✕
        </button>
      </div>

      <!-- 主题列表（同构 THEME 清单渲染，doc/18） -->
      <div class="mt-4 space-y-3">
        <template v-for="t in THEMES" :key="t.id">
          <!-- 已上线主题 -->
          <div
            v-if="t.available"
            @click="t.current ? null : switchTo(t)"
            class="p-3.5 rounded-xl flex items-start gap-3 relative transition-all duration-150 group"
            :class="t.current
              ? 'border-2.5 border-toy-black bg-toy-yellow/20 cursor-default'
              : 'border-2 border-toy-black bg-white hover:bg-sky-50 cursor-pointer'"
            :style="{ boxShadow: '3px 3px 0px #121212' }"
          >
            <div class="w-10 h-10 rounded-lg border-2 border-toy-black flex items-center justify-center text-xl shrink-0 group-hover:scale-105" :class="t.current ? 'bg-toy-yellow' : 'bg-sky-200'">
              {{ t.icon }}
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="font-extrabold text-sm text-toy-black">{{ t.name }}</span>
                <span
                  class="px-2 py-0.5 text-[10px] font-black border border-toy-black rounded-full"
                  :class="t.current ? 'bg-toy-mint text-toy-black' : 'bg-toy-gray text-toy-muted'"
                >
                  {{ t.current ? '当前启用' : '一键切换' }}
                </span>
              </div>
              <p class="text-xs text-toy-muted font-medium mt-1">{{ t.desc }}</p>
            </div>
          </div>

          <!-- 未上线主题预留插槽 -->
          <div
            v-else
            class="p-3.5 rounded-xl border-2 border-dashed border-toy-muted/60 bg-gray-50/70 flex items-start gap-3 opacity-80"
          >
            <div class="w-10 h-10 rounded-lg bg-gray-200 border-2 border-gray-400 flex items-center justify-center text-xl shrink-0">
              {{ t.icon }}
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="font-bold text-sm text-gray-700">{{ t.name }}</span>
                <span class="px-2 py-0.5 text-[10px] font-bold bg-gray-200 text-gray-600 rounded-full">即将推出</span>
              </div>
              <p class="text-xs text-gray-500 font-medium mt-1">{{ t.desc }}</p>
            </div>
          </div>
        </template>
      </div>

      <!-- 底部提示 -->
      <div class="mt-5 pt-3 border-t-2 border-toy-gray flex items-center justify-between">
        <span class="text-xs font-bold text-toy-muted">
          切换记录已持久化至 LocalStorage
        </span>
        <button
          @click="$emit('close')"
          class="toy-btn px-4 py-1.5 bg-toy-yellow text-xs rounded-lg"
        >
          确定
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { THEMES, savePreferredTheme, type ThemeEntry } from '../theme';

defineProps<{
  isOpen: boolean;
}>();

defineEmits<{
  (e: 'close'): void;
}>();

function switchTo(t: ThemeEntry) {
  savePreferredTheme(t.id);
  // dev 跨端口联调（toy 5174 / classic 5173）保留；生产同源直接跳挂载路径
  if (window.location.port === '5174' && t.id === 'classic') {
    window.location.href = `http://${window.location.hostname}:5173`;
    return;
  }
  if (window.location.port === '5173' && t.id === 'toy') {
    window.location.href = `http://${window.location.hostname}:5174`;
    return;
  }
  window.location.href = t.path;
}
</script>
