<script setup>
// 审计日志：登录 / 节点变更 / 设置变更 / 功耗校准全记录，分页展示。
// 来源 IP 仅展示哈希（对应后端 source_ip_hash 设计）。
import { ref, onMounted, computed } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { adminApi, fmtTs } from './store'

const items = ref([])
const total = ref(0)
const page = ref(1)
const size = 20
const loading = ref(true)
const filter = ref('')

const ACTION_META = {
  setup: { label: '初始化', cls: 'ok' },
  login: { label: '登录', cls: 'ok' },
  login_fail: { label: '登录失败', cls: 'warn' },
  logout: { label: '登出', cls: '' },
  password: { label: '改密', cls: '' },
  server_create: { label: '新增节点', cls: 'ok' },
  server_update: { label: '编辑节点', cls: '' },
  server_delete: { label: '删除节点', cls: 'warn' },
  server_order: { label: '节点排序', cls: '' },
  power_calibrate: { label: '功耗校准', cls: 'ok' },
  settings: { label: '设置', cls: '' },
  install: { label: '安装', cls: '' },
}

const filtered = computed(() => {
  if (!filter.value) return items.value
  const q = filter.value.toLowerCase()
  return items.value.filter(
    (x) =>
      (ACTION_META[x.action]?.label || x.action).includes(q) ||
      (x.detail || '').toLowerCase().includes(q) ||
      (x.actor || '').includes(q)
  )
})

const pages = computed(() => Math.max(1, Math.ceil(total.value / size)))

async function load() {
  loading.value = true
  const r = await adminApi.listAudit(page.value, size)
  items.value = r.items
  total.value = r.total
  loading.value = false
}

function go(p) {
  page.value = Math.min(pages.value, Math.max(1, p))
  load()
}

onMounted(load)
</script>

<template>
  <div class="audit-admin">
    <div class="panel-toolbar">
      <div class="toolbar-info">
        共 {{ total }} 条记录 · 来源 IP 仅以哈希存储展示（SHA-256）
      </div>
      <input v-model="filter" type="search" class="search-input" placeholder="筛选操作 / 内容 / 操作者" />
    </div>

    <div class="glass-card audit-card">
      <div v-if="loading" class="empty-card">加载中…</div>
      <div v-else-if="!filtered.length" class="empty-card">没有匹配的记录</div>
      <table v-else class="audit-table">
        <thead>
          <tr>
            <th>时间</th>
            <th>操作</th>
            <th>对象</th>
            <th>详情</th>
            <th>操作者</th>
            <th>来源 IP</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="x in filtered" :key="x.id">
            <td class="mono nowrap">{{ fmtTs(x.ts) }}</td>
            <td><span class="chip" :class="`chip-${ACTION_META[x.action]?.cls || 'gray'}`">{{ ACTION_META[x.action]?.label || x.action }}</span></td>
            <td class="mono nowrap">{{ x.target }}</td>
            <td class="audit-detail">{{ x.detail }}</td>
            <td class="nowrap">{{ x.actor }}</td>
            <td class="mono fp">{{ x.source_ip_hash || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="pages > 1" class="audit-pager">
      <button class="btn btn-ghost btn-sm" :disabled="page <= 1" @click="go(page - 1)">上一页</button>
      <span class="pager-info">{{ page }} / {{ pages }}</span>
      <button class="btn btn-ghost btn-sm" :disabled="page >= pages" @click="go(page + 1)">下一页</button>
    </div>
  </div>
</template>
