// ============================================================
// BeaconTower · 企业级路由表
// - 管理端改为嵌套路由：/admin/servers|settings|security|audit（可深链/刷新保持）
// - 全局守卫：初始化→setup；未登录→login；已登录访问login→servers
// - 通配符→独立 404 页（禁止静默回首页）
// ============================================================
import { createRouter, createWebHistory } from 'vue-router'
import AppShell from '../layouts/AppShell.vue'
import OverviewView from '../views/monitor/OverviewView.vue'
import AdminLayout from '../views/admin/AdminLayout.vue'
import ServersView from '../views/admin/ServersView.vue'
import SettingsView from '../views/admin/SettingsView.vue'
import SecurityView from '../views/admin/SecurityView.vue'
import AuditView from '../views/admin/AuditView.vue'
import LoginView from '../views/admin/LoginView.vue'
import SetupView from '../views/admin/SetupView.vue'
import NotFoundView from '../views/error/NotFoundView.vue'
import { adminClient } from '../api/admin'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: AppShell,
      children: [
        {
          path: '',
          name: 'overview',
          component: OverviewView,
          meta: { breadcrumb: [{ label: '总览' }] },
        },
        {
          path: 'admin/setup',
          name: 'admin-setup',
          component: SetupView,
          meta: { public: true, breadcrumb: [{ label: '管理面板', to: '/admin/servers' }, { label: '初始化' }] },
        },
        {
          path: 'admin/login',
          name: 'admin-login',
          component: LoginView,
          meta: { public: true, breadcrumb: [{ label: '管理面板', to: '/admin/servers' }, { label: '登录' }] },
        },
        {
          path: 'admin',
          component: AdminLayout,
          meta: { requiresAuth: true },
          children: [
            { path: '', redirect: '/admin/servers' },
            {
              path: 'servers',
              name: 'admin-servers',
              component: ServersView,
              meta: {
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: '节点管理' }],
              },
            },
            {
              path: 'settings',
              name: 'admin-settings',
              component: SettingsView,
              meta: {
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: '采集与展示' }],
              },
            },
            {
              path: 'security',
              name: 'admin-security',
              component: SecurityView,
              meta: {
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: '安全与账号' }],
              },
            },
            {
              path: 'audit',
              name: 'admin-audit',
              component: AuditView,
              meta: {
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: '审计日志' }],
              },
            },
          ],
        },
        {
          path: ':pathMatch(.*)*',
          name: 'not-found',
          component: NotFoundView,
          meta: { breadcrumb: [{ label: '页面不存在' }] },
        },
      ],
    },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

let statusCache = null
let statusAt = 0

async function getStatus() {
  // 30 秒内复用，避免每次跳转都打一次 status 接口
  if (statusCache && Date.now() - statusAt < 30_000) return statusCache
  try {
    statusCache = await adminClient.status()
    statusAt = Date.now()
    return statusCache
  } catch {
    return { initialized: true, loggedIn: false }
  }
}

export function __resetRouteCache() {
  statusCache = null
  statusAt = 0
}

router.beforeEach(async (to) => {
  if (to.meta.public || to.name === 'not-found' || to.name === 'overview') return true

  if (to.meta.requiresAuth || to.path.startsWith('/admin')) {
    const st = await getStatus()
    if (!st.initialized) return { path: '/admin/setup' }
    if (!st.loggedIn) return { path: '/admin/login', query: { redirect: to.fullPath } }
    return true
  }
  return true
})

// 登录/初始化页：已登录用户直接进管理端，避免重复登录
router.beforeEach(async (to) => {
  if (to.name === 'admin-login' || to.name === 'admin-setup') {
    const st = await getStatus()
    if (to.name === 'admin-login' && st.initialized && st.loggedIn) return { path: '/admin/servers' }
    if (to.name === 'admin-setup' && st.initialized) {
      return { path: st.loggedIn ? '/admin/servers' : '/admin/login' }
    }
  }
  return true
})

export default router
