<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm">
    <div 
      class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 relative overflow-hidden"
      :style="{ boxShadow: '6px 6px 0px #121212' }"
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
        >
          ✕
        </button>
      </div>

      <!-- 主题列表 -->
      <div class="mt-4 space-y-3">
        <!-- 1. 当前玩具风格 -->
        <div 
          class="p-3.5 rounded-xl border-2.5 border-toy-black bg-toy-yellow/20 flex items-start gap-3 relative cursor-pointer"
          :style="{ boxShadow: '3px 3px 0px #121212' }"
        >
          <div class="w-10 h-10 rounded-lg bg-toy-yellow border-2 border-toy-black flex items-center justify-center text-xl shrink-0">
            🧸
          </div>
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="font-extrabold text-sm text-toy-black">Toy Neubrutalism · 玩具新野兽派</span>
              <span class="px-2 py-0.5 text-[10px] font-black bg-toy-mint text-toy-black border border-toy-black rounded-full">
                当前启用
              </span>
            </div>
            <p class="text-xs text-toy-muted font-medium mt-1">
              纯黑物理注塑外框、高对比度积木明黄配色、趣味像素机器人与清晰运维大字。
            </p>
          </div>
        </div>

        <!-- 2. 老版经典拟态风格 -->
        <div 
          @click="switchToClassic"
          class="p-3.5 rounded-xl border-2 border-toy-black bg-white hover:bg-sky-50 flex items-start gap-3 relative cursor-pointer transition-all duration-150 group"
          :style="{ boxShadow: '3px 3px 0px #121212' }"
        >
          <div class="w-10 h-10 rounded-lg bg-sky-200 border-2 border-toy-black flex items-center justify-center text-xl shrink-0 group-hover:scale-105">
            🪟
          </div>
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="font-extrabold text-sm text-toy-black">Classic Glass · 经典天空玻璃拟态</span>
              <span class="px-2 py-0.5 text-[10px] font-bold bg-toy-gray text-toy-muted border border-toy-black rounded-full">
                一键切换
              </span>
            </div>
            <p class="text-xs text-toy-muted font-medium mt-1">
              经典亮色拟态界面，极光渐变背景与半透明玻璃卡片设计。
            </p>
          </div>
        </div>

        <!-- 3. 未来可扩展插槽示例 -->
        <div 
          class="p-3.5 rounded-xl border-2 border-dashed border-toy-muted/60 bg-gray-50/70 flex items-start gap-3 opacity-80"
        >
          <div class="w-10 h-10 rounded-lg bg-gray-200 border-2 border-gray-400 flex items-center justify-center text-xl shrink-0">
            ✨
          </div>
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="font-bold text-sm text-gray-700">更多主题风格正在制作</span>
              <span class="px-2 py-0.5 text-[10px] font-bold bg-gray-200 text-gray-600 rounded-full">
                即将推出
              </span>
            </div>
            <p class="text-xs text-gray-500 font-medium mt-1">
              未来将陆续推出复古极客终端、赛博霓虹等更多视觉主题，敬请期待！
            </p>
          </div>
        </div>
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
defineProps<{
  isOpen: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

function switchToClassic() {
  localStorage.setItem('beacontower_preferred_theme', 'classic');
  // 如果本地处于不同端口（如老前端跑在 5173，玩具版在 5174），跳转到老前端
  if (window.location.port === '5174') {
    window.location.href = `http://${window.location.hostname}:5173`;
  } else {
    window.location.href = '/?theme=classic';
  }
}
</script>
