import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': '/src' },
  },
  server: {
    host: true,
    port: 5173,
    // 开发期前后端分离：API 转发到本地 Go 后端（M1 起监听 8080）
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        // ECharts 单独分包：图表库升级不影响业务包 hash，利于长缓存
        manualChunks: {
          echarts: ['echarts/core', 'echarts/charts', 'echarts/components', 'echarts/renderers'],
          vendor: ['vue', 'vue-router', 'pinia'],
        },
      },
    },
  },
})
