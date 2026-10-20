import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import path from 'path';
import { writeFileSync, mkdirSync } from 'node:fs';

const keepDistPlaceholder = {
  name: 'bt-keep-dist-placeholder',
  closeBundle() {
    mkdirSync('dist', { recursive: true });
    writeFileSync('dist/.placeholder', '');
  },
};

// https://vitejs.dev/config/
export default defineConfig({
  // v2.9 起 toy 主题迁至 /toy/（Sky Glass 占根路径）：资源、favicon、boot 脚本
  // 全部按此前缀生成，与后端 router 的非默认主题作用域对齐。
  base: '/toy/',
  plugins: [vue(), keepDistPlaceholder],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5174,
    host: '0.0.0.0',
    // 开发期前后端分离：API 转发（默认本地 Go 后端，可用 TOY_PROXY_TARGET 覆盖）
    proxy: {
      '/api': {
        target: process.env.TOY_PROXY_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
});
