<script setup>
// 节点管理：列表（状态/画像/功耗能力/最近采集）+ 添加/编辑（含试连回读）+ 删除。
// 试连成功即回读系统画像、geo 预览、host key 指纹与 RAPL 功耗能力探测。
import { ref, onMounted, computed } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { adminApi, agoFromTs, fmtTs } from './store'
import { fmtWatts, fmtSizeShort } from '../utils/format'

const servers = ref([])
const loading = ref(true)
const showEditor = ref(false)
const editing = ref(null) // null=新增，否则为节点对象
const confirmDelete = ref(null)
const expand = ref(0) // 展开详情的节点 id（0 = 无）

async function load() {
  loading.value = true
  servers.value = await adminApi.listServers()
  loading.value = false
}

function openCreate() {
  editing.value = null
  showEditor.value = true
}

function openEdit(s) {
  editing.value = s
  showEditor.value = true
}

async function remove(id) {
  await adminApi.deleteServer(id)
  confirmDelete.value = null
  await load()
}

async function toggleEnabled(s) {
  await adminApi.updateServer(s.id, { enabled: !s.enabled })
  await load()
}

async function move(s, dir) {
  const ids = servers.value.map((x) => x.id)
  const i = ids.indexOf(s.id)
  const j = i + dir
  if (j < 0 || j >= ids.length) return
  ;[ids[i], ids[j]] = [ids[j], ids[i]]
  await adminApi.reorder(ids)
  await load()
}

const sorted = computed(() => [...servers.value].sort((a, b) => a.sort_order - b.sort_order))

const powerBadge = (s) => {
  if (!s.profile) return { cls: 'na', text: '未试连' }
  const p = s.power
  if (!p?.rapl) return { cls: 'na', text: '不支持 RAPL' }
  const src = p.base_load_source === 'calibration' ? '电池校准' : p.base_load_source === 'manual' ? '人工设定' : '默认'
  return { cls: 'ok', text: `基础功耗 ${fmtWatts(p.base_load_w)} · ${src}` }
}

onMounted(load)
</script>

<template>
  <div class="servers-admin">
    <div class="panel-toolbar">
      <div class="toolbar-info">
        共 {{ servers.length }} 个节点 ·
        <span class="tip">试连成功后自动回读系统画像、公网 IP 定位与 RAPL 功耗能力；IP 仅管理端可见</span>
      </div>
      <button class="btn btn-primary" @click="openCreate">
        <AppIcon name="plus" class="btn-ico" />添加节点
      </button>
    </div>

    <div v-if="loading" class="glass-card empty-card">加载中…</div>
    <div v-else-if="!servers.length" class="glass-card empty-card">
      还没有节点，点击右上角「添加节点」开始 —— 只需填 SSH 地址与账号密码，其余自动回读。
    </div>

    <div v-for="s in sorted" :key="s.id" class="glass-card server-row" :class="{ disabled: !s.enabled }">
      <div class="row-main">
        <div class="row-id">
          <span class="row-icon"><AppIcon name="server" /></span>
          <button class="row-updown" title="上移" @click="move(s, -1)">↑</button>
          <button class="row-updown" title="下移" @click="move(s, 1)">↓</button>
        </div>

        <div class="row-titles">
          <div class="row-name">
            {{ s.name }}
            <span v-if="s.hidden" class="chip chip-gray">隐藏</span>
            <span v-if="!s.enabled" class="chip chip-gray">已暂停</span>
          </div>
          <div class="row-sub">
            <span class="mono">{{ s.ssh.username }}@{{ s.ssh.host }}:{{ s.ssh.port }}</span>
            <span class="dot-sep">·</span>
            <span>{{ s.ssh.auth_type === 'key' ? '密钥登录' : '密码登录' }}</span>
            <span class="dot-sep">·</span>
            <span>{{ s.profile ? `${s.profile.os_name} ${s.profile.os_version} · ${s.profile.arch} · ${s.profile.cpu_cores}核` : '画像未回读' }}</span>
          </div>
        </div>

        <div class="row-power" :title="s.power?.last_error || ''">
          <AppIcon name="bolt" class="row-power-ico" :class="powerBadge(s).cls" />
          <span class="row-power-text" :class="powerBadge(s).cls">{{ powerBadge(s).text }}</span>
        </div>

        <div class="row-collect">
          <span class="row-collect-label">最近采集</span>
          <span :class="s.power?.last_error ? 'text-red' : 'text-ok'">{{ agoFromTs(s.last_success_at) }}</span>
        </div>

        <div class="row-actions">
          <button class="btn btn-ghost btn-sm" @click="expand = expand === s.id ? 0 : s.id">
            {{ expand === s.id ? '收起' : '详情' }}
          </button>
          <button class="btn btn-ghost btn-sm" @click="openEdit(s)"><AppIcon name="edit" class="btn-ico" />编辑</button>
          <button class="btn btn-ghost btn-sm" :class="{ 'text-red': !s.enabled }" @click="toggleEnabled(s)">
            {{ s.enabled ? '暂停' : '启用' }}
          </button>
          <button class="btn btn-ghost btn-sm text-red" @click="confirmDelete = s">
            <AppIcon name="trash" class="btn-ico" />删除
          </button>
        </div>
      </div>

      <!-- 展开详情：画像 / geo / 指纹 / 功耗 / 私有备注（仅管理端可见字段） -->
      <div v-if="expand === s.id" class="row-detail">
        <div class="detail-grid">
          <div class="detail-item">
            <span class="detail-label">主机名（私有）</span>
            <span class="mono">{{ s.profile?.hostname || '—' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">公网 IP（私有）</span>
            <span class="mono">{{ s.profile?.public_ip || '—' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">地区来源</span>
            <span>{{ s.region_source === 'auto' ? 'IP 自动定位' : '管理员指定' }} · {{ s.region || '未定位' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">内核</span>
            <span class="mono">{{ s.profile?.kernel || '—' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">CPU</span>
            <span>{{ s.profile?.cpu_model || '—' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">内存 / 磁盘</span>
            <span>{{ s.profile ? `${fmtSizeShort(s.profile.mem_total / 1024 ** 3)} / ${fmtSizeShort(s.profile.disk_total / 1024 ** 3)}` : '—' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">Host Key 指纹</span>
            <span class="mono fp">{{ s.ssh.host_key_fp || '首次连接时记录（TOFU）' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">添加时间</span>
            <span>{{ fmtTs(s.created_at) }}</span>
          </div>
        </div>
        <div v-if="s.power?.last_error" class="detail-error">
          <AppIcon name="warn" class="err-ico" />最近采集失败：{{ s.power.last_error }}
        </div>
        <div v-if="s.note_private" class="detail-note">
          <span class="detail-label">私有备注（访客不可见）</span>
          <p>{{ s.note_private }}</p>
        </div>
      </div>
    </div>

    <!-- 编辑/新增弹层 -->
    <ServerEditor
      v-if="showEditor"
      :server="editing"
      @close="showEditor = false"
      @saved="((showEditor = false), load())"
    />

    <!-- 删除确认 -->
    <div v-if="confirmDelete" class="modal-mask" @click.self="confirmDelete = null">
      <div class="glass-card modal-card">
        <h3 class="modal-title">删除节点「{{ confirmDelete.name }}」？</h3>
        <p class="modal-desc">将级联删除该节点的 SSH 凭据、系统画像与全部历史指标，此操作不可恢复。</p>
        <div class="modal-actions">
          <button class="btn btn-ghost" @click="confirmDelete = null">取消</button>
          <button class="btn btn-danger" @click="remove(confirmDelete.id)">确认删除</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import ServerEditor from './ServerEditor.vue'
export default {
  components: { ServerEditor },
}
</script>
