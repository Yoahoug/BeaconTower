// ============================================================
// BeaconTower · 企业级路由表
// - 管理端改为嵌套路由：/admin/servers|settings|security|audit（可深链/刷新保持）
// - 管理入口隐藏：公开 UI（侧栏/顶栏/404）不设任何管理链接，仅直接访问 /admin 进入
// - 全局守卫：初始化→setup；未登录→login；已登录访问login→servers
// - 通配符→独立 404 页（禁止静默回首页）
// - 管理子页懒加载：公开首屏不含管理代码（隐藏接口 + 首屏体积）
// ============================================================
import { createRouter, createWebHistory } from 'vue-router'
import AppShell from '../layouts/AppShell.vue'
import OverviewView from '../views/monitor/OverviewView.vue'
import NotFoundView from '../views/error/NotFoundView.vue'
import { getCachedStatus } from '../api/auth'

// 节点详情（公开只读，首屏直载：总览卡片点击即进，无需懒加载等待）
import ServerDetailView from '../views/monitor/ServerDetailView.vue'

// 穿透状态（公开只读，二级页）：懒加载，公开首屏 bundle 不含它
const TunnelView = () => import('../views/monitor/TunnelView.vue')

// WG 组网状态（公开只读，二级页）：懒加载，公开首屏 bundle 不含它
const MeshPublicView = () => import('../views/monitor/MeshPublicView.vue')

// 管理端按路由懒加载：公开页 bundle 不包含节点/凭据等管理代码。
const AdminLayout = () => import('../views/admin/AdminLayout.vue')
const ServersView = () => import('../views/admin/ServersView.vue')
const MeshView = () => import('../views/admin/MeshView.vue')
const FrpView = () => import('../views/admin/FrpView.vue')
const SettingsView = () => import('../views/admin/SettingsView.vue')
const SecurityView = () => import('../views/admin/SecurityView.vue')
const AuditView = () => import('../views/admin/AuditView.vue')
const LoginView = () => import('../views/admin/LoginView.vue')
const SetupView = () => import('../views/admin/SetupView.vue')

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
          path: 'server/:id',
          name: 'server-detail',
          component: ServerDetailView,
          props: true,
          meta: { breadcrumb: [{ label: '总览', to: '/' }, { label: '节点详情' }] },
        },
        {
          // 公开只读：只下发字段白名单内的用量与在用节点健康度（doc/13 §8）
          path: 'tunnels',
          name: 'tunnels',
          component: TunnelView,
          meta: { public: true, breadcrumb: [{ label: '总览', to: '/' }, { label: '穿透状态' }] },
        },
        {
          // 公开只读：只下发组网健康度 / 成员在线 / 中转流量（doc/04 §1.6）
          path: 'mesh',
          name: 'mesh-public',
          component: MeshPublicView,
          meta: { public: true, breadcrumb: [{ label: '总览', to: '/' }, { label: '组网状态' }] },
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
                title: '节点管理',
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: '节点管理' }],
              },
            },
            {
              path: 'mesh',
              name: 'admin-mesh',
              component: MeshView,
              meta: {
                title: 'WG 组网',
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: 'WG 组网' }],
              },
            },
            {
              path: 'frp',
              name: 'admin-frp',
              component: FrpView,
              meta: {
                title: '内网穿透',
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: '内网穿透' }],
              },
            },
            {
              path: 'settings',
              name: 'admin-settings',
              component: SettingsView,
              meta: {
                title: '采集与展示',
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: '采集与展示' }],
              },
            },
            {
              path: 'security',
              name: 'admin-security',
              component: SecurityView,
              meta: {
                title: '安全与账号',
                breadcrumb: [{ label: '总览', to: '/' }, { label: '管理面板' }, { label: '安全与账号' }],
              },
            },
            {
              path: 'audit',
              name: 'admin-audit',
              component: AuditView,
              meta: {
                title: '审计日志',
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

async function getStatus() {
  // 30 秒内复用，避免每次跳转都打一次 status 接口（缓存实现见 api/auth.js）
  try {
    return await getCachedStatus()
  } catch {
    // status 拿不到结论（网络抖动/后端瞬时 5xx）时不能假装「已初始化且未登录」，
    // 否则在线管理员会被误踢到登录页；抛出去让导航失败，恢复后重试即可
    throw new Error('登录状态检查失败')
  }
}

router.beforeEach(async (to) => {
  if (to.meta.public || to.name === 'not-found' || to.name === 'overview' || to.name === 'server-detail') return true

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
