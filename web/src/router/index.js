import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from '../views/DashboardView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: DashboardView },
    {
      path: '/admin',
      name: 'admin',
      // 管理面板：初始化向导 → 登录 → 节点/设置/安全/审计（隐藏后台，无公开入口）
      component: () => import('../views/AdminPanelView.vue'),
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

export default router
