<script setup>
// 节点编辑器（新增/编辑共用）：
// - 基本信息（名称/地区/标签/备注/隐藏）
// - SSH 连接（地址/端口/用户名/密码或密钥；编辑时留空 = 保留原值）
// - 一键试连：回读画像 + geo + host key 指纹 + RAPL 功耗能力预览
// - 功耗校准：设置平台基础功耗（对齐 power-monitor 的 BASE_LOAD 语义）
import { ref, computed } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { adminApi } from './store'
import { fmtWatts } from '../utils/format'

const props = defineProps({
  server: { type: Object, default: null }, // null = 新增
})
const emit = defineEmits(['close', 'saved'])

const isEdit = computed(() => !!props.server)

const form = ref({
  name: props.server?.name || '',
  region: props.server?.region || '',
  tagsText: (props.server?.tags || []).join(', '),
  note_public: props.server?.note_public || '',
  note_private: props.server?.note_private || '',
  hidden: props.server?.hidden || false,
  host: props.server?.ssh?.host || '',
  port: props.server?.ssh?.port || 22,
  sshUsername: props.server?.ssh?.username || '',
  authType: props.server?.ssh?.auth_type || 'password',
  password: '',
  privateKey: '',
  passphrase: '',
  baseLoadW: props.server?.power?.base_load_w || 0,
})

const probing = ref(false)
const probe = ref(props.server?.profile
  ? {
      ok: true,
      latency_ms: null,
      profile: props.server.profile,
      geo: { public_ip: props.server.profile.public_ip, country: props.server.profile.geo_country, city: props.server.profile.geo_city },
      host_key_fp: props.server.ssh.host_key_fp,
      cached: true,
    }
  : null)
const probeError = ref('')
const saving = ref(false)
const saveError = ref('')

async function runProbe() {
  probeError.value = ''
  probing.value = true
  try {
    probe.value = await adminApi.testConnection({
      host: form.value.host,
      port: form.value.port,
      username: form.value.sshUsername,
      auth_type: form.value.authType,
      password: form.value.password || undefined,
      private_key: form.value.privateKey || undefined,
    })
  } catch (e) {
    probe.value = null
    probeError.value = e.message
  } finally {
    probing.value = false
  }
}

async function save() {
  saveError.value = ''
  if (!form.value.name) return (saveError.value = '请填写节点名称')
  if (!form.value.host || !form.value.sshUsername) return (saveError.value = '请填写 SSH 地址与用户名')
  if (!isEdit.value && form.value.authType === 'password' && !form.value.password) {
    return (saveError.value = '请填写 SSH 密码')
  }
  if (!isEdit.value && form.value.authType === 'key' && !form.value.privateKey) {
    return (saveError.value = '请粘贴 SSH 私钥')
  }
  saving.value = true
  try {
    const payload = {
      name: form.value.name,
      region: form.value.region,
      tags: form.value.tagsText.split(/[,，]/).map((t) => t.trim()).filter(Boolean),
      note_public: form.value.note_public,
      note_private: form.value.note_private,
      hidden: form.value.hidden,
      ssh: {
        host: form.value.host,
        port: Number(form.value.port) || 22,
        username: form.value.sshUsername,
        auth_type: form.value.authType,
        // 留空 = 保留原值（凭据不回显原则）
        password: form.value.password || undefined,
        private_key: form.value.privateKey || undefined,
        passphrase: form.value.passphrase || undefined,
      },
      probe: probe.value && !probe.value.cached ? probe.value : null,
    }
    if (isEdit.value) {
      await adminApi.updateServer(props.server.id, payload)
      // 功耗校准值随编辑保存
      if (form.value.baseLoadW > 0 && props.server.power?.rapl) {
        await adminApi.recalibratePower(props.server.id, Number(form.value.baseLoadW))
      }
    } else {
      const { id } = await adminApi.createServer(payload)
      if (form.value.baseLoadW > 0 && probe.value?.profile?.power?.rapl) {
        await adminApi.recalibratePower(id, Number(form.value.baseLoadW))
      }
    }
    emit('saved')
  } catch (e) {
    saveError.value = e.message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="modal-mask" @click.self="emit('close')">
    <div class="glass-card modal-card editor-card">
      <div class="editor-head">
        <h3 class="modal-title">{{ isEdit ? `编辑节点 · ${server.name}` : '添加节点' }}</h3>
        <button class="btn btn-ghost btn-sm" @click="emit('close')">关闭</button>
      </div>

      <div class="editor-body">
        <!-- 基本信息 -->
        <section class="editor-sec">
          <h4 class="sec-title">基本信息</h4>
          <div class="form-grid">
            <label class="field">
              <span class="field-label">节点名称 <i class="req">*</i></span>
              <input v-model="form.name" type="text" placeholder="如：香港 · 轻量 01" />
            </label>
            <label class="field">
              <span class="field-label">地区（留空 = 按公网 IP 自动定位）</span>
              <input v-model="form.region" type="text" placeholder="香港" />
            </label>
            <label class="field field-wide">
              <span class="field-label">标签（逗号分隔）</span>
              <input v-model="form.tagsText" type="text" placeholder="Web, 主力" />
            </label>
            <label class="field">
              <span class="field-label">公开备注（访客可见）</span>
              <input v-model="form.note_public" type="text" placeholder="可留空" />
            </label>
            <label class="field">
              <span class="field-label">私有备注（仅管理端）</span>
              <input v-model="form.note_private" type="text" placeholder="用途 / 账单 / 到期日" />
            </label>
            <label class="check-field field-wide">
              <input v-model="form.hidden" type="checkbox" />
              <span>在公开页隐藏此节点（仅管理端可见）</span>
            </label>
          </div>
        </section>

        <!-- SSH 连接 -->
        <section class="editor-sec">
          <h4 class="sec-title">SSH 连接 <span class="sec-note">凭据加密存储，读取接口永不回显；编辑时留空 = 保留原值</span></h4>
          <div class="form-grid">
            <label class="field field-addr">
              <span class="field-label">地址 <i class="req">*</i></span>
              <input v-model="form.host" type="text" placeholder="IP 或域名" :disabled="isEdit && !!probe?.cached" />
            </label>
            <label class="field field-port">
              <span class="field-label">端口</span>
              <input v-model="form.port" type="number" min="1" max="65535" />
            </label>
            <label class="field">
              <span class="field-label">用户名 <i class="req">*</i></span>
              <input v-model="form.sshUsername" type="text" placeholder="root" />
            </label>
            <div class="field">
              <span class="field-label">认证方式</span>
              <div class="seg-control">
                <button type="button" class="seg" :class="{ active: form.authType === 'password' }" @click="form.authType = 'password'">密码</button>
                <button type="button" class="seg" :class="{ active: form.authType === 'key' }" @click="form.authType = 'key'">私钥</button>
              </div>
            </div>
            <label v-if="form.authType === 'password'" class="field field-wide">
              <span class="field-label">{{ isEdit ? '密码（留空保留原值）' : '密码' }}</span>
              <input v-model="form.password" type="password" autocomplete="new-password" placeholder="••••••••" />
            </label>
            <template v-else>
              <label class="field field-wide">
                <span class="field-label">{{ isEdit ? '私钥（留空保留原值）' : '私钥（OpenSSH 格式）' }}</span>
                <textarea v-model="form.privateKey" rows="3" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" class="mono" />
              </label>
              <label class="field">
                <span class="field-label">私钥口令（可空）</span>
                <input v-model="form.passphrase" type="password" autocomplete="off" />
              </label>
            </template>
          </div>

          <div class="probe-row">
            <button class="btn" type="button" :disabled="probing || !form.host || !form.sshUsername" @click="runProbe">
              <AppIcon name="refresh" class="btn-ico" :class="{ spin: probing }" />
              {{ probing ? '正在试连…' : '测试连接并回读画像' }}
            </button>
            <span v-if="probeError" class="form-error inline"><AppIcon name="warn" class="err-ico" />{{ probeError }}</span>
          </div>

          <!-- 试连结果 -->
          <div v-if="probe" class="probe-result" :class="{ cached: probe.cached }">
            <div class="probe-ok">
              <AppIcon name="check" class="ok-ico" />
              <span v-if="probe.cached">已保存的画像（修改连接信息后重新试连可刷新）</span>
              <span v-else>连接成功 · 延迟 {{ probe.latency_ms }}ms</span>
            </div>
            <div class="detail-grid">
              <div class="detail-item"><span class="detail-label">主机名</span><span class="mono">{{ probe.profile.hostname }}</span></div>
              <div class="detail-item"><span class="detail-label">系统</span><span>{{ probe.profile.os_name }} {{ probe.profile.os_version }} · {{ probe.profile.arch }}</span></div>
              <div class="detail-item"><span class="detail-label">CPU</span><span>{{ probe.profile.cpu_model }} · {{ probe.profile.cpu_cores }}核</span></div>
              <div class="detail-item"><span class="detail-label">内存 / 磁盘</span><span>{{ (probe.profile.mem_total / 1024 ** 3).toFixed(0) }}G / {{ (probe.profile.disk_total / 1024 ** 3).toFixed(0) }}G</span></div>
              <div class="detail-item"><span class="detail-label">虚拟化</span><span>{{ probe.profile.virt }}</span></div>
              <div class="detail-item"><span class="detail-label">公网 IP（私有）</span><span class="mono">{{ probe.geo?.public_ip || '—' }}{{ probe.geo ? ` · ${probe.geo.country}` : '' }}</span></div>
              <div class="detail-item"><span class="detail-label">Host Key 指纹</span><span class="mono fp">{{ probe.host_key_fp || '—' }}</span></div>
              <div class="detail-item">
                <span class="detail-label">功耗能力</span>
                <span :class="probe.profile.power?.rapl ? 'text-ok' : 'text-3'">
                  {{ probe.profile.power?.rapl ? `RAPL 可用 · 基础功耗 ${fmtWatts(probe.profile.power.base_load_w)}` : '未暴露 RAPL 功率计（AMD/虚拟机常见）' }}
                  {{ probe.profile.power?.battery ? ' · 含电池（可自动校准）' : '' }}
                </span>
              </div>
            </div>
          </div>
        </section>

        <!-- 功耗校准（RAPL 可用才显示） -->
        <section v-if="probe?.profile?.power?.rapl" class="editor-sec">
          <h4 class="sec-title">功耗校准 <span class="sec-note">整机功耗 = CPU + 内存 + 基础功耗；有电池的机器放电时自动校准，也可在此手动设定</span></h4>
          <div class="form-grid">
            <label class="field field-port">
              <span class="field-label">基础功耗（W）</span>
              <input v-model.number="form.baseLoadW" type="number" min="0" max="200" step="0.1" />
            </label>
            <div class="field">
              <span class="field-label">当前来源</span>
              <div class="seg-static">{{ probe.profile.power.base_load_source === 'calibration' ? '电池自动校准' : '默认估算' }}</div>
            </div>
          </div>
        </section>

        <div v-if="saveError" class="form-error"><AppIcon name="warn" class="err-ico" />{{ saveError }}</div>
      </div>

      <div class="modal-actions editor-foot">
        <button class="btn btn-ghost" @click="emit('close')">取消</button>
        <button class="btn btn-primary" :disabled="saving" @click="save">
          {{ saving ? '保存中…' : isEdit ? '保存修改' : '保存并添加' }}
        </button>
      </div>
    </div>
  </div>
</template>
