import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [
    vue(),
    // PWA（doc/15）：Workbox 预缓存 app shell，/api 全走网络绝不缓存。
    // registerType autoUpdate：新版本下载后下次导航生效，不打扰使用中用户。
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['robots.txt'],
      manifest: false, // manifest 用 public/manifest.webmanifest 手写版（字段语义可见）
      workbox: {
        // 预缓存：构建产物全量入 precache（hash 文件名，天然内容寻址）
        globPatterns: ['**/*.{js,css,html,png,svg,ico,webmanifest,woff2}'],
        navigateFallback: 'index.html',
        navigateFallbackDenylist: [/^\/api\//],
        runtimeCaching: [
          // /api/* 一律 NetworkOnly（NetworkFirst 也不行：断网时会把旧缓存
          // 当新鲜数据展示，监控数据宁缺毋假）。SSE 由 EventSource 直连，
          // 天然不经 SW。
          {
            urlPattern: ({ url }) => url.pathname.startsWith('/api/'),
            handler: 'NetworkOnly',
          },
        ],
      },
    }),
  ],
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
