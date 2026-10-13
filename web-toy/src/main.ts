import { createApp } from 'vue';
import './style.css';
import App from './App.vue';
import { initApi } from './services/api';

// mock 门面是异步加载的（生产构建 tree-shake 掉仿真引擎）：必须先完成门面装配再挂载，
// 否则 proxy 的首次取用会把 apiInstance 钉死在 real 上、VITE_API_MODE=mock 被静默忽略。
// 用 IIFE 而非顶层 await：构建目标不支持顶层 await。
async function boot() {
  try {
    await initApi();
  } catch {
    // 门面装配失败也要挂载：视图自身的失败态会如实提示（比白屏可诊断）
  }
  createApp(App).mount('#app');
}

void boot();
