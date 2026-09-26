<!-- ============================================================
     节点管理（v2.0）：工具条 + 玻璃行列表（画像/功耗能力/最近采集）
     + 展开详情。编辑器与删除确认带 spring 弹窗转场。
     ============================================================ -->
<script setup>
import { ref, computed, onMounted } from 'vue'
import AppIcon from '../../components/AppIcon.vue'
import StateEmpty from '../../components/ui/StateEmpty.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import ServerEditorModal from './ServerEditorModal.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import { adminClient } from '../../api/admin'
import { agoFromTs, fmtTs } from '../../api/auth'
import { useAdminStore } from '../../stores/admin'
import { useUiStore } from '../../stores/ui'
import { fmtWatts, fmtSizeShort } from '../../utils/format'

const admin = useAdminStore()
const ui = useUiStore()

const showEditor = ref(false)
const editing = ref(null)
const confirmDelete = ref(null)
const deleting = ref(false)
const expandedId = ref(0)
const relocatingId = ref(0)

const sorted = computed(() =>
  [...admin.servers].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0)),
)

function powerBadge(s) {
  if (!s.profile) return { cls: 'bt-tag--outline', text: '未试连' }
  const p = s.power
  if (!p?.rapl) return { cls: 'bt-tag--outline', text: '不支持 RAPL' }
  const src =
    p.base_load_source === 'calibration' ? '电池校准' : p.base_load_source === 'manual' ? '人工设定' : '默认'
  return { cls: 'bt-tag--info', text: `基础功耗 ${fmtWatts(p.base_load_w)} · ${src}` }
}

function openCreate() {
  editing.value = null
  showEditor.value = true
}

function openEdit(s) {
  editing.value = s
  showEditor.value = true
}

function onSaved() {
  showEditor.value = false
  ui.notify('节点已保存')
  admin.loadServers()
}

async function remove() {
  if (!confirmDelete.value) return
  deleting.value = true
  try {
    await adminClient.deleteServer(confirmDelete.value.id)
    confirmDelete.value = null
    ui.notify('节点已删除')
    await admin.loadServers()
  } finally {
    deleting.value = false
  }
}

async function toggleEnabled(s) {
  try {
    await adminClient.updateServer(s.id, { enabled: !s.enabled })
    ui.notify(s.enabled ? '节点已暂停' : '节点已启用')
    await admin.loadServers()
  } catch (e) {
    ui.notify(e?.message || '操作失败')
  }
}

async function move(s, dir) {
  const ids = sorted.value.map((x) => x.id)
  const i = ids.indexOf(s.id)
  const j = i + dir
  if (j < 0 || j >= ids.length) return
  ;[ids[i], ids[j]] = [ids[j], ids[i]]
  try {
    await adminClient.reorder(ids)
    await admin.loadServers()
  } catch (e) {
    ui.notify(e?.message || '排序失败')
  }
}

async function relocate(s) {
  if (relocatingId.value) return
  relocatingId.value = s.id
  try {
    const r = await adminClient.relocate(s.id)
    ui.notify(`已重新定位：${r?.region || s.region || '—'}`)
    await admin.loadServers()
  } catch (e) {
    ui.notify(e?.message || '重新定位失败')
  } finally {
    relocatingId.value = 0
  }
}

onMounted(() => {
  admin.loadServers()
})
</script>

<template>
  <div>
    <div class="bt-toolbar">
      <div class="bt-toolbar__info">
        共 {{ admin.servers.length }} 个节点 · 试连成功后自动回读系统画像、公网 IP 定位与 RAPL 功耗能力；IP 仅管理端可见
      </div>
      <div class="bt-toolbar__actions">
        <button class="bt-btn bt-btn--primary" type="button" @click="openCreate">
          <AppIcon name="plus" aria-hidden="true" />添加节点
        </button>
      </div>
    </div>

    <StateSkeleton v-if="admin.serversLoading" :rows="4" />
    <StateError v-else-if="admin.serversError" :message="admin.serversError" @retry="admin.loadServers()" />
    <div v-else-if="!admin.servers.length" class="bt-card">
      <StateEmpty title="还没有节点" desc="点击右上角「添加节点」开始 —— 只需填 SSH 地址与账号，其余自动回读。">
        <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="openCreate">
          <AppIcon name="plus" aria-hidden="true" />添加节点
        </button>
      </StateEmpty>
    </div>

    <TransitionGroup v-else name="list" tag="div" class="server-rows">
      <section v-for="s in sorted" :key="s.id" class="server-row" :class="{ 'is-disabled': !s.enabled }" :aria-label="`节点 ${s.name}`">
        <div class="server-row__main">
          <div class="server-row__move" aria-label="调整排序">
            <button type="button" title="上移" aria-label="上移" @click="move(s, -1)">▲</button>
            <button type="button" title="下移" aria-label="下移" @click="move(s, 1)">▼</button>
          </div>
          <span class="server-row__icon" aria-hidden="true"><AppIcon name="server" /></span>

          <div class="server-row__titles">
            <div class="server-row__name">
              {{ s.name }}
              <span v-if="s.is_self" class="bt-tag bt-tag--info">本机</span>
              <span v-if="s.hidden" class="bt-tag">隐藏</span>
              <span v-if="!s.enabled" class="bt-tag bt-tag--warning">已暂停</span>
            </div>
            <div class="server-row__sub">
              <span class="mono">{{ s.is_self ? '本机进程采集 · 免 SSH' : `${s.ssh.username}@${s.ssh.host}:${s.ssh.port}` }}</span>
              <span aria-hidden="true">·</span>
              <span v-if="!s.is_self">{{ s.ssh.auth_type === 'key' ? '密钥登录' : '密码登录' }}</span>
              <span v-if="!s.is_self" aria-hidden="true">·</span>
              <span>{{ s.profile ? `${s.profile.os_name} ${s.profile.os_version} · ${s.profile.arch} · ${s.profile.cpu_cores}核` : '画像未回读' }}</span>
            </div>
          </div>

          <div class="server-row__power" :title="s.power?.last_error || ''">
            <AppIcon name="bolt" aria-hidden="true" />
            <span class="bt-tag" :class="powerBadge(s).cls">{{ powerBadge(s).text }}</span>
          </div>

          <div class="server-row__collect">
            <small>最近采集</small>
            <span :class="s.power?.last_error ? 'bt-text-danger' : 'bt-text-ok'" class="tnum">
              {{ agoFromTs(s.last_success_at) }}
            </span>
          </div>

          <div class="server-row__actions">
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :aria-expanded="expandedId === s.id" @click="expandedId = expandedId === s.id ? 0 : s.id">
              {{ expandedId === s.id ? '收起' : '详情' }}
            </button>
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="openEdit(s)">
              <AppIcon name="edit" aria-hidden="true" />编辑
            </button>
            <button v-if="!s.is_self" class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="relocatingId === s.id" @click="relocate(s)">
              <AppIcon name="refresh" :class="{ 'is-spin': relocatingId === s.id }" aria-hidden="true" />{{ relocatingId === s.id ? '定位中…' : '重定位' }}
            </button>
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :class="{ 'bt-text-danger': s.enabled }" @click="toggleEnabled(s)">
              {{ s.enabled ? '暂停' : '启用' }}
            </button>
            <button v-if="!s.is_self" class="bt-btn bt-btn--ghost bt-btn--sm bt-text-danger" type="button" @click="confirmDelete = s">
              <AppIcon name="trash" aria-hidden="true" />删除
            </button>
          </div>
        </div>

        <Transition name="page-sub">
          <div v-if="expandedId === s.id" class="server-row__detail">
            <dl class="bt-def-grid">
              <div class="bt-def"><dt>主机名（私有）</dt><dd class="mono">{{ s.profile?.hostname || '—' }}</dd></div>
              <div class="bt-def"><dt>公网 IP（私有）</dt><dd class="mono">{{ s.profile?.public_ip || '—' }}</dd></div>
              <div class="bt-def"><dt>地区来源</dt><dd>{{ s.region_source === 'auto' ? 'IP 自动定位' : '管理员指定' }} · {{ s.region || '未定位' }}</dd></div>
              <div class="bt-def"><dt>内核</dt><dd class="mono">{{ s.profile?.kernel || '—' }}</dd></div>
              <div class="bt-def"><dt>CPU</dt><dd>{{ s.profile?.cpu_model || '—' }}</dd></div>
              <div class="bt-def">
                <dt>内存 / 磁盘</dt>
                <dd>{{ s.profile ? `${fmtSizeShort(s.profile.mem_total / 1024 ** 3)} / ${fmtSizeShort(s.profile.disk_total / 1024 ** 3)}` : '—' }}</dd>
              </div>
              <div class="bt-def"><dt>Host Key 指纹</dt><dd class="mono">{{ s.ssh.host_key_fp || '首次连接时记录（TOFU）' }}</dd></div>
              <div class="bt-def"><dt>添加时间</dt><dd class="tnum">{{ fmtTs(s.created_at) }}</dd></div>
            </dl>
            <div v-if="s.power?.last_error" class="detail-error" role="alert">
              <AppIcon name="warn" aria-hidden="true" />最近采集失败：{{ s.power.last_error }}
            </div>
            <div v-if="s.note_private" class="detail-note">
              <span class="bt-text-muted" style="font-size: 12px">私有备注（访客不可见）</span>
              <p>{{ s.note_private }}</p>
            </div>
          </div>
        </Transition>
      </section>
    </TransitionGroup>

    <Transition name="modal">
      <ServerEditorModal v-if="showEditor" :server="editing" @close="showEditor = false" @saved="onSaved" />
    </Transition>

    <Transition name="modal">
      <ConfirmDialog
        v-if="confirmDelete"
        title="删除节点"
        :message="`确认删除「${confirmDelete.name}」？将级联删除其 SSH 凭据、系统画像与全部历史指标，此操作不可恢复。`"
        confirm-text="确认删除"
        :busy="deleting"
        danger
        @cancel="confirmDelete = null"
        @confirm="remove"
      />
    </Transition>
  </div>
</template>
