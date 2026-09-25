// BeaconTower v2.0 · 应用入口
// 样式引入顺序：令牌 → 基础 → 动效 → 组件 → 骨架 → 页面（禁止乱序）。
// pinia 必须在 router 之前 use，保证守卫可读 store。
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { vSpotlight } from './composables/useSpotlight'
import './styles/tokens.css'
import './styles/base.css'
import './styles/motion.css'
import './styles/components.css'
import './styles/layout.css'
import './styles/views.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
// v-spotlight：指针辉光（写入 --mx/--my，配合 .spot::before 渐变）
app.directive('spotlight', vSpotlight)
app.mount('#app')
