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
    <!-- 能耗：闪电 -->
    <template v-else-if="name === 'bolt'">
      <path d="M8.9 1.6 3.6 9h3.2l-.9 5.4L11.5 7H8.2l.7-5.4z" />
    </template>
    <!-- 温度：温度计 -->
    <template v-else-if="name === 'temp'">
      <path d="M6.7 9.4V3.2a1.3 1.3 0 0 1 2.6 0v6.2a2.9 2.9 0 1 1-2.6 0z" />
      <path d="M8 7.2v4.4" />
    </template>
    <!-- 设置：滑杆 -->
    <template v-else-if="name === 'sliders'">
      <path d="M2.2 4.6h6.4M11.4 4.6h2.4M2.2 11.4h2.4M7.4 11.4h6.4" />
      <circle cx="9.9" cy="4.6" r="1.7" />
      <circle cx="6.1" cy="11.4" r="1.7" />
    </template>
    <!-- 钥匙：登录 / 凭据 -->
    <template v-else-if="name === 'key'">
      <circle cx="5.4" cy="5.4" r="3.2" />
      <path d="m7.8 7.8 5.6 5.6M11.2 11.2l1.6-1.6M12.9 12.9l1.5-1.5" />
    </template>
    <!-- 盾牌：安全 / 审计 -->
    <template v-else-if="name === 'shield'">
      <path d="M8 1.6 3 3.4v4.2c0 3.1 2.1 5.4 5 6.8 2.9-1.4 5-3.7 5-6.8V3.4L8 1.6z" />
      <path d="m5.9 7.9 1.5 1.5 2.8-2.9" />
    </template>
    <!-- 加号：新建节点 -->
    <template v-else-if="name === 'plus'">
      <path d="M8 3.2v9.6M3.2 8h9.6" />
    </template>
    <!-- 编辑：铅笔 -->
    <template v-else-if="name === 'edit'">
      <path d="m3.1 12.9.5-3L10.9 3.6a1.5 1.5 0 0 1 2.1 0l-.6-.6a1.5 1.5 0 0 1 0 2.1l-6.3 6.3-3 .5z" />
    </template>
    <!-- 删除：垃圾桶 -->
    <template v-else-if="name === 'trash'">
      <path d="M2.8 4.4h10.4M6.4 4.4V3a1 1 0 0 1 1-1h1.2a1 1 0 0 1 1 1v1.4M4.2 4.4l.6 8.2a1.2 1.2 0 0 0 1.2 1.1h4a1.2 1.2 0 0 0 1.2-1.1l.6-8.2" />
      <path d="M6.6 7v3.8M9.4 7v3.8" />
    </template>
    <!-- 刷新 -->
    <template v-else-if="name === 'refresh'">
      <path d="M13.2 8a5.2 5.2 0 1 1-1.6-3.7M13.4 1.9v2.6h-2.6" />
    </template>
    <!-- 插头 / 电源：功耗来源 -->
    <template v-else-if="name === 'plug'">
      <path d="M5.4 1.8v3M10.6 1.8v3M4 4.8h8v1.8a4 4 0 0 1-8 0V4.8z" />
      <path d="M8 10.6v3.6" />
    </template>
    <!-- 电池：电池校准 -->
    <template v-else-if="name === 'battery'">
      <rect x="1.8" y="5" width="10.6" height="6" rx="1.6" />
      <path d="M14.4 7v2" />
      <path d="M4 7.2v1.6M6.4 7.2v1.6" />
    </template>
    <!-- 历史：时钟 -->
    <template v-else-if="name === 'clock'">
      <circle cx="8" cy="8" r="6.2" />
      <path d="M8 4.8V8l2.2 1.6" />
    </template>
    <!-- 返回箭头 -->
    <template v-else-if="name === 'arrow-left'">
      <path d="M13 8H3.4M7 3.8 3 8l4 4.2" />
    </template>
    <!-- 菜单（移动端抽屉） -->
    <template v-else-if="name === 'menu'">
      <path d="M2.4 4.2h11.2M2.4 8h11.2M2.4 11.8h11.2" />
    </template>
    <!-- 搜索 -->
    <template v-else-if="name === 'search'">
      <circle cx="7.2" cy="7.2" r="4.7" />
      <path d="m10.8 10.8 3.4 3.4" />
    </template>
    <!-- 空态：服务器 -->
    <template v-else-if="name === 'empty'">
      <rect x="2" y="3" width="12" height="10" rx="1.6" />
      <path d="M2 6.4h12M5 4.7h.01M6.8 4.7h.01M5 11.2h6" />
    </template>
    <!-- 下载 / 导出 -->
    <template v-else-if="name === 'download'">
      <path d="M8 2.4v7.4M4.8 6.8 8 10l3.2-3.2M2.8 13.2h10.4" />
    </template>
    <!-- 用户：管理员账号 -->
    <template v-else-if="name === 'user'">
      <circle cx="8" cy="5.4" r="2.9" />
      <path d="M2.8 13.8c.8-2.6 2.8-3.9 5.2-3.9s4.4 1.3 5.2 3.9" />
    </template>
    <!-- 对勾 -->
    <template v-else-if="name === 'check'">
      <path d="m3.2 8.4 3.2 3.2 6.4-6.8" />
    </template>
    <!-- 警示 -->
    <template v-else-if="name === 'warn'">
      <path d="M8 2.2 1.8 13h12.4L8 2.2z" />
      <path d="M8 6.4v3M8 11.4h.01" />
    </template>
    <!-- 日志：文档 -->
    <template v-else-if="name === 'log'">
      <path d="M4 1.8h5.4L12.8 5v9.2H4V1.8z" />
      <path d="M9.2 1.8V5h3.6M6.2 8h3.6M6.2 10.8h3.6" />
    </template>
    <template v-else-if="name === 'apple'">
      <path d="M9.1 3.2c.7-.8 1.2-1.9 1-3-1 .1-2 .7-2.7 1.5-.6.7-1.1 1.8-1 2.9 1.1 0 2.1-.6 2.7-1.4z" fill="currentColor" stroke="none" />
      <path d="M11.9 8.5c0-1.7 1.4-2.5 1.5-2.6-.8-1.2-2.1-1.4-2.5-1.4-1.1-.1-2.1.6-2.7.6-.6 0-1.4-.6-2.3-.6-1.2 0-2.3.7-2.9 1.8-1.2 2.1-.3 5.3.9 7 .6.9 1.3 1.8 2.2 1.8.9 0 1.2-.6 2.3-.6s1.4.6 2.3.6c1 0 1.6-.9 2.2-1.7.7-1 1-2 1-2.1-.1 0-2-.8-2-2.8z" fill="currentColor" stroke="none" />
    </template>
    <!-- v2.0 新增：吞吐方向 -->
    <template v-else-if="name === 'arrow-up'">
      <path d="M8 12.8V3.2M4.4 6.8 8 3.2l3.6 3.6" />
    </template>
    <template v-else-if="name === 'arrow-down'">
      <path d="M8 3.2v9.6M4.4 9.2 8 12.8l3.6-3.6" />
    </template>
    <!-- CPU 芯片 -->
    <template v-else-if="name === 'cpu'">
      <rect x="4" y="4" width="8" height="8" rx="1.6" />
      <rect x="6.4" y="6.4" width="3.2" height="3.2" rx="0.8" />
      <path d="M6.4 1.8v1.4M9.6 1.8v1.4M6.4 12.8v1.4M9.6 12.8v1.4M1.8 6.4h1.4M1.8 9.6h1.4M12.8 6.4h1.4M12.8 9.6h1.4" />
    </template>
    <!-- 内存条 -->
    <template v-else-if="name === 'memory'">
      <rect x="1.8" y="5" width="12.4" height="6" rx="1.2" />
      <path d="M4.4 5V3.8M8 5V3.8M11.6 5V3.8M4.4 12.2V11M8 12.2V11M11.6 12.2V11" />
      <path d="M4.6 7.2v1.6M8 7.2v1.6M11.4 7.2v1.6" />
    </template>
    <!-- 磁盘 -->
    <template v-else-if="name === 'disk'">
      <circle cx="8" cy="8" r="6.2" />
      <circle cx="8" cy="8" r="1.8" />
      <path d="M8 1.8v2.4" />
    </template>
    <!-- 地球：地区 / 节点数 -->
    <template v-else-if="name === 'globe'">
      <circle cx="8" cy="8" r="6.2" />
      <path d="M1.8 8h12.4M8 1.8c-1.7 1.6-2.6 3.7-2.6 6.2s.9 4.6 2.6 6.2c1.7-1.6 2.6-3.7 2.6-6.2S9.7 3.4 8 1.8z" />
    </template>
    <!-- 波动：吞吐 / 活动 -->
    <template v-else-if="name === 'activity'">
      <path d="M1.8 8h2.6L6.4 4l3.2 8 2-5.4 1.2 1.4h1.4" />
    </template>
    <!-- 仪表：负载 -->
    <template v-else-if="name === 'gauge'">
      <path d="M2.5 12.6a6.4 6.4 0 1 1 11 0" />
      <path d="M8 8.6 10.8 5" />
      <circle cx="8" cy="9.2" r="1.2" />
    </template>
    <!-- 闪电簇：能耗 -->
    <template v-else-if="name === 'sparkles'">
      <path d="M8 2.2 8.9 5l2.8.9-2.8.9L8 9.6 7.1 6.8 4.3 5.9l2.8-.9L8 2.2z" />
      <path d="M11.8 9.8l.55 1.65 1.65.55-1.65.55-.55 1.65-.55-1.65-1.65-.55 1.65-.55.55-1.65z" />
    </template>
    <!-- 层叠：总量 -->
    <template v-else-if="name === 'layers'">
      <path d="m8 1.8 6 3.2-6 3.2-6-3.2 6-3.2z" />
      <path d="m2 8.6 6 3.2 6-3.2M2 11.8l6 3.2 6-3.2" />
    </template>
    <!-- 退出登录 -->
    <template v-else-if="name === 'logout'">
      <path d="M9.5 2.6H4.2a1.2 1.2 0 0 0-1.2 1.2v8.4a1.2 1.2 0 0 0 1.2 1.2h5.3" />
      <path d="M11 5.2 13.8 8 11 10.8M13.8 8H6.4" />
    </template>
    <!-- 日历：月度 -->
    <template v-else-if="name === 'calendar'">
      <rect x="2" y="3" width="12" height="11" rx="1.8" />
      <path d="M2 6.6h12M5.4 1.8V4M10.6 1.8V4" />
    </template>
  </svg>
</template>

<script setup>
defineProps({
  name: { type: String, required: true },
})
</script>
