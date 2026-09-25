// 管理面板壳：路由守卫按初始化/登录状态分流（初始化向导 → 登录 → 子页）。
// 子页用内部标签页切换，保持单页玻璃卡片布局，不引入嵌套路由。
<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { adminApi } from '../admin/store'
import WizardAuth from '../admin/WizardAuth.vue'
import ServersAdmin from '../admin/ServersAdmin.vue'
import SettingsAdmin from '../admin/SettingsAdmin.vue'
import SecurityAdmin from '../admin/SecurityAdmin.vue'
import AuditAdmin from '../admin/AuditAdmin.vue'

const router = useRouter()
const loading = ref(true)
const authed = ref(false)
const tab = ref('servers')

const tabs = [
  { key: 'servers', label: '节点管理', icon: 'server' },
  { key: 'settings', label: '采集与展示', icon: 'sliders' },
  { key: 'security', label: '安全与账号', icon: 'key' },
  { key: 'audit', label: '审计日志', icon: 'log' },
]

async function check() {
  loading.value = true
  const st = await adminApi.status()
  authed.value = st.loggedIn
  loading.value = false
}

async function onAuthed() {
  authed.value = true
}

async function logout() {
  await adminApi.logout()
  authed.value = false
  router.push('/')
}

onMounted(check)
</script>

<template>
  <main class="page admin-page">
    <!-- 初始化向导 / 登录 -->
    <WizardAuth v-if="!loading && !authed" @authed="onAuthed" />

    <template v-else-if="!loading && authed">
      <div class="page-head admin-head">
        <div>
          <h1>管理面板</h1>
          <p class="admin-head-sub">SSH 凭据加密存储 · 公开页永不返回敏感字段 · 本原型数据保存在浏览器本地</p>
        </div>
        <div class="admin-head-actions">
          <button class="btn btn-ghost" @click="logout">
            <AppIcon name="key" class="btn-ico" />退出登录
          </button>
        </div>
      </div>

      <nav class="admin-tabs">
        <button
          v-for="t in tabs"
          :key="t.key"
          class="admin-tab"
          :class="{ active: tab === t.key }"
          @click="tab = t.key"
        >
          <AppIcon :name="t.icon" class="admin-tab-ico" />
          <span>{{ t.label }}</span>
        </button>
      </nav>

      <div class="admin-body">
        <ServersAdmin v-if="tab === 'servers'" />
        <SettingsAdmin v-else-if="tab === 'settings'" />
        <SecurityAdmin v-else-if="tab === 'security'" />
        <AuditAdmin v-else-if="tab === 'audit'" />
      </div>
    </template>

    <div v-else class="admin-loading">
      <div class="glass-card loading-card">正在加载…</div>
    </div>
  </main>
</template>
