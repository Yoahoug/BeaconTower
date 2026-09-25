<template>
  <!-- 线性界面符号：16 画布 / stroke 1.5 / currentColor，零 emoji -->
  <svg
    viewBox="0 0 16 16"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <template v-if="name === 'dashboard'">
      <rect x="1.5" y="1.5" width="5.4" height="5.4" rx="1.4" />
      <rect x="9.1" y="1.5" width="5.4" height="5.4" rx="1.4" />
      <rect x="1.5" y="9.1" width="5.4" height="5.4" rx="1.4" />
      <rect x="9.1" y="9.1" width="5.4" height="5.4" rx="1.4" />
    </template>
    <template v-else-if="name === 'server'">
      <rect x="1.5" y="2.5" width="13" height="4.6" rx="1.4" />
      <rect x="1.5" y="8.9" width="13" height="4.6" rx="1.4" />
      <path d="M4.2 4.8h.01M4.2 11.2h.01" stroke-width="1.8" />
    </template>
    <template v-else-if="name === 'pulse'">
      <path d="M1.5 8h3l1.8-4.6 3 9L11.5 8h3" />
    </template>
    <!-- 品牌徽标：信标/灯塔信号 -->
    <template v-else-if="name === 'beacon'">
      <path d="M8 1.2v2.2" />
      <path d="M3.8 3.2l1.4 1.4M12.2 3.2l-1.4 1.4" />
      <path d="M2 13.2c.9-1.7 1.6-3.4 1.6-5A4.4 4.4 0 0 1 8 3.8a4.4 4.4 0 0 1 4.4 4.4c0 1.6.7 3.3 1.6 5" />
      <path d="M3.4 13.4h9.2" />
    </template>
    <!-- Linux 企鹅（简笔轮廓） -->
    <template v-else-if="name === 'linux'">
      <path d="M8 1.6c-1.9 0-2.9 1.4-2.9 3.2 0 1-.1 1.8-.6 2.7-.7 1.2-1.9 2.7-1.9 4.2 0 1 .6 1.7 1.4 1.7.6 0 .9-.2 1.4-.2.8 0 1 .9 2.6.9s1.8-.9 2.6-.9c.5 0 .8.2 1.4.2.8 0 1.4-.7 1.4-1.7 0-1.5-1.2-3-1.9-4.2-.5-.9-.6-1.7-.6-2.7 0-1.8-1-3.2-2.9-3.2z" />
      <path d="M6.4 5.2c.4-.5.9-.7 1.6-.7s1.2.2 1.6.7" />
      <path d="M6.5 7.2c.5.4 1 .5 1.5.5s1-.1 1.5-.5" />
    </template>
    <!-- 定位：地区由公网 IP 自动解析 -->
    <template v-else-if="name === 'pin'">
      <path d="M8 14.2C5.1 11.2 3.3 8.9 3.3 6.7a4.7 4.7 0 0 1 9.4 0c0 2.2-1.8 4.5-4.7 7.5z" />
      <circle cx="8" cy="6.7" r="1.7" />
    </template>
    <template v-else-if="name === 'apple'">
      <path d="M9.1 3.2c.7-.8 1.2-1.9 1-3-1 .1-2 .7-2.7 1.5-.6.7-1.1 1.8-1 2.9 1.1 0 2.1-.6 2.7-1.4z" fill="currentColor" stroke="none" />
      <path d="M11.9 8.5c0-1.7 1.4-2.5 1.5-2.6-.8-1.2-2.1-1.4-2.5-1.4-1.1-.1-2.1.6-2.7.6-.6 0-1.4-.6-2.3-.6-1.2 0-2.3.7-2.9 1.8-1.2 2.1-.3 5.3.9 7 .6.9 1.3 1.8 2.2 1.8.9 0 1.2-.6 2.3-.6s1.4.6 2.3.6c1 0 1.6-.9 2.2-1.7.7-1 1-2 1-2.1-.1 0-2-.8-2-2.8z" fill="currentColor" stroke="none" />
    </template>
  </svg>
</template>

<script setup>
defineProps({
  name: { type: String, required: true },
})
</script>
