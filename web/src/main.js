// BeaconTower · 应用入口
// 约束：全局样式按“令牌→基础→组件→骨架→页面”顺序引入；
// pinia 必须在 router 之前 use，保证守卫可读 store（如需）。
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './styles/tokens.css'
import './styles/base.css'
import './styles/components.css'
import './styles/layout.css'
import './styles/views.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
