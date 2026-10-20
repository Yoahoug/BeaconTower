import { writeFileSync, mkdirSync } from 'node:fs'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'

// dist/.placeholder 是 go:embed 编译期存在性检查的兜底文件（.gitignore 反选保留）。
// vite 每次构建都会清空 outDir，把它一起删掉，于是「构建完 git 就显示删了一个入库
// 文件」，稍不留神 add -A 就把它提交掉了。这里在产物写完后补回，保持仓库自洽。
const keepDistPlaceholder = {
  name: 'bt-keep-dist-placeholder',
  closeBundle() {
    mkdirSync('dist', { recursive: true })
    writeFileSync('dist/.placeholder', '')
  },
}

export default defineConfig({
  // v2.9 起 Sky Glass（classic）成为根路径默认主题：资源、SW、manifest 的 URL
  // 全部按根作用域生成，与后端 router 的默认主题作用域（/assets、/sw.js…）对齐。
  // toy 迁至 /toy/（自身 base '/toy/'）；旧 /classic/* 地址由后端 302 回根路径。
  base: '/',
  plugins: [
    vue(),
    keepDistPlaceholder,
    // PWA（doc/15，v2.8.3 修订）：injectManifest 自管 SW（src/sw.js）——
    // 导航 NetworkFirst（在线永远拿最新壳，离线回退预缓存）+ install 期
    // skipWaiting。generateSW 的默认形态会把壳做成「只认预缓存」，新 SW 一旦
    // 停在 waiting，用户就被旧界面钉死（v2.8.1→v2.8.2 线上实测）。
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['robots.txt'],
      manifest: false, // manifest 用 public/manifest.webmanifest 手写版（字段语义可见）
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.js',
      injectManifest: {
        // 预缓存：构建产物全量入 precache（hash 文件名，天然内容寻址）
        globPatterns: ['**/*.{js,css,html,png,svg,ico,webmanifest,woff2}'],
        // iife 经典脚本：模块化 SW 需要 Safari 16.4+，iife 覆盖面最大
        rollupFormat: 'iife',
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
