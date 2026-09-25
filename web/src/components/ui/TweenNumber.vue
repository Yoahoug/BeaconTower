// 数字补间展示组件：数值变化时沿 easeOutExpo 平滑滚动。
// 用法：<TweenNumber :value="summary.watts" :format="fmtWatts" />
<script setup>
import { computed } from 'vue'
import { useTween } from '../../composables/useTween'

const props = defineProps({
  value: { type: Number, default: 0 },
  // 展示格式化：复用 utils/format.js 的同一批函数，禁止组件内拼单位
  format: { type: Function, default: (v) => String(Math.round(v)) },
  duration: { type: Number, default: 800 },
})

const display = useTween(() => props.value, { duration: props.duration })
const text = computed(() => props.format(display.value))
</script>

<template>
  <span class="tnum">{{ text }}</span>
</template>
