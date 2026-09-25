<!-- 审计日志：筛选 + 玻璃表格（sticky 表头/横向滚动）+ 分页 -->
<script setup>
import { computed, onMounted } from 'vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import StateEmpty from '../../components/ui/StateEmpty.vue'
import { useAdminStore } from '../../stores/admin'
import { fmtTs } from '../../api/auth'

const admin = useAdminStore()

const ACTION_META = {
  setup: { label: '初始化', cls: 'bt-tag--success' },
  login: { label: '登录', cls: 'bt-tag--success' },
  login_fail: { label: '登录失败', cls: 'bt-tag--warning' },
  logout: { label: '登出', cls: '' },
  password: { label: '改密', cls: '' },
  server_create: { label: '新增节点', cls: 'bt-tag--success' },
  server_update: { label: '编辑节点', cls: '' },
  server_delete: { label: '删除节点', cls: 'bt-tag--warning' },
  server_order: { label: '节点排序', cls: '' },
  server_locate: { label: '重新定位', cls: 'bt-tag--info' },
  power_calibrate: { label: '功耗校准', cls: 'bt-tag--info' },
  settings: { label: '设置', cls: '' },
  install: { label: '安装', cls: '' },
}

const rows = computed(() => admin.filteredAudit)

function go(p) {
  admin.loadAudit(Math.min(admin.auditPages, Math.max(1, p)))
}

onMounted(() => {
  admin.loadAudit(1)
})
</script>

<template>
  <div>
    <div class="bt-toolbar" role="search">
      <div class="bt-toolbar__info">共 {{ admin.auditTotal }} 条记录 · 来源 IP 仅以哈希存储展示（SHA-256）</div>
      <div class="bt-toolbar__actions">
        <input
          :value="admin.auditFilter"
          type="search"
          class="bt-input"
          style="max-width: 260px"
          placeholder="筛选操作 / 内容 / 操作者"
          aria-label="筛选审计日志"
          @input="admin.setAuditFilter($event.target.value)"
        />
      </div>
    </div>

    <StateSkeleton v-if="admin.auditLoading" :rows="6" />
    <StateError v-else-if="admin.auditError" :message="admin.auditError" @retry="admin.loadAudit(admin.auditPage)" />
    <div v-else-if="!rows.length" class="bt-card">
      <StateEmpty title="没有匹配的记录" desc="尝试更换筛选关键词。" />
    </div>
    <Transition v-else name="page-sub" appear>
      <div class="bt-table-wrap">
        <table class="bt-table">
          <caption>审计日志 · 第 {{ admin.auditPage }} / {{ admin.auditPages }} 页</caption>
          <thead>
            <tr>
              <th scope="col">时间</th>
              <th scope="col">操作</th>
              <th scope="col">对象</th>
              <th scope="col">详情</th>
              <th scope="col">操作者</th>
              <th scope="col">来源 IP</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="x in rows" :key="x.id">
              <td class="mono tnum" style="white-space: nowrap">{{ fmtTs(x.ts) }}</td>
              <td><span class="bt-tag" :class="ACTION_META[x.action]?.cls || ''">{{ ACTION_META[x.action]?.label || x.action }}</span></td>
              <td class="mono" style="white-space: nowrap">{{ x.target }}</td>
              <td style="min-width: 220px">{{ x.detail }}</td>
              <td style="white-space: nowrap">{{ x.actor }}</td>
              <td class="mono">{{ x.source_ip_hash || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </Transition>

    <div v-if="admin.auditPages > 1" class="bt-pager">
      <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="admin.auditPage <= 1" @click="go(admin.auditPage - 1)">上一页</button>
      <span class="bt-pager__info tnum">{{ admin.auditPage }} / {{ admin.auditPages }}</span>
      <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="admin.auditPage >= admin.auditPages" @click="go(admin.auditPage + 1)">下一页</button>
    </div>
  </div>
</template>
