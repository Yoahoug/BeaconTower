import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { tilt } from './directives/tilt'
import './styles/main.css'

createApp(App).use(router).directive('tilt', tilt).mount('#app')
