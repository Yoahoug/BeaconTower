<!-- ============================================================
     节点编辑器（新增/编辑共用，大弹窗）· v2.0 玻璃弹窗
     a11y：role=dialog + labelledby + ESC 关闭 + 初始聚焦首个输入。
     凭据不回显：编辑时留空 = 保留原值。
     新增模式带草稿：误关弹窗自动暂存 sessionStorage（仅当前标签页），
     重开自动恢复；保存成功或手动"清空重填"后失效。
     ============================================================ -->
<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import AppIcon from '../../components/AppIcon.vue'
import { adminClient } from '../../api/admin'
import { fmtWatts } from '../../utils/format'

const props = defineProps({
  server: { type: Object, default: null }, // null = 新增
})
const emit = defineEmits(['close', 'saved'])

const isEdit = computed(() => !!props.server)
const isSelf = computed(() => !!props.server?.is_self)

// ---- 新增模式草稿缓存（sessionStorage：关标签页才清，误关弹窗可恢复）----
const DRAFT_KEY = 'bt-server-draft'
const restoredFromDraft = ref(false)

function readDraft() {
  try {
    return JSON.parse(sessionStorage.getItem(DRAFT_KEY) || 'null')
  } catch {
    return null
  }
}

function writeDraft() {
  if (isEdit.value) return
  try {
    const { password: _pw, privateKey: _pk, passphrase: _pp, ...safe } = form.value
    sessionStorage.setItem(
      DRAFT_KEY,
      JSON.stringify({ form: safe, probe: probe.value }),
    )
  } catch {
    /* 存储不可用时静默降级为无缓存 */
  }
}

function clearDraft() {
  try {
    sessionStorage.removeItem(DRAFT_KEY)
  } catch {
    /* ignore */
  }
  restoredFromDraft.value = false
}

function draftForm() {
  return {
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
  }
}

const form = ref(draftForm())

// 新增模式下恢复草稿（sessionStorage 仅本标签页可见）
const draft = !isEdit.value ? readDraft() : null
if (draft?.form && (draft.form.host || draft.form.name)) {
  Object.assign(form.value, draft.form)
  restoredFromDraft.value = true
}

const probing = ref(false)
const probe = ref(
  (() => {
    if (props.server?.profile) {
      return {
        ok: true,
        latency_ms: null,
        profile: props.server.profile,
        geo: {
          public_ip: props.server.profile.public_ip,
          country: props.server.profile.geo_country,
          city: props.server.profile.geo_city,
        },
        host_key_fp: props.server.ssh.host_key_fp,
        cached: true,
      }
    }
    // 新增模式：恢复草稿里最近一次试连结果（凭据已丢弃，仅画像/指纹）
    if (draft?.probe?.profile && !draft.probe.cached && (draft.form?.host || draft.form?.name)) {
      return { ...draft.probe, cached: false }
    }
    return null
  })(),
)
const connChanged = computed(() => {
  if (!props.server?.ssh) return false
  return (
    form.value.host !== props.server.ssh.host ||
    Number(form.value.port) !== Number(props.server.ssh.port) ||
    form.value.sshUsername !== props.server.ssh.username
  )
})
const probeError = ref('')
const saving = ref(false)
const saveError = ref('')
const firstInput = ref(null)

// 任何字段变化都自动续存草稿（新增模式）
watch(
  form,
  writeDraft,
  { deep: true },
)

async function runProbe() {
  probeError.value = ''
  probing.value = true
  try {
    probe.value = await adminClient.testConnection({
      host: form.value.host,
      port: form.value.port,
      username: form.value.sshUsername,
      auth_type: form.value.authType,
      password: form.value.password || undefined,
      private_key: form.value.privateKey || undefined,
      passphrase: form.value.passphrase || undefined,
    })
    writeDraft()
  } catch (e) {
    probe.value = null
    probeError.value = e?.message || '试连失败'
  } finally {
    probing.value = false
  }
}

async function save() {
  saveError.value = ''
  if (!form.value.name) return (saveError.value = '请填写节点名称')
  if (!isSelf.value) {
    if (!form.value.host || !form.value.sshUsername) return (saveError.value = '请填写 SSH 地址与用户名')
    if (!isEdit.value && form.value.authType === 'password' && !form.value.password) {
      return (saveError.value = '请填写 SSH 密码')
    }
    if (!isEdit.value && form.value.authType === 'key' && !form.value.privateKey) {
      return (saveError.value = '请粘贴 SSH 私钥')
    }
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
    }
    if (!isSelf.value) {
      payload.ssh = {
        host: form.value.host,
        port: Number(form.value.port) || 22,
        username: form.value.sshUsername,
        auth_type: form.value.authType,
        password: form.value.password || undefined,
        private_key: form.value.privateKey || undefined,
        passphrase: form.value.passphrase || undefined,
      }
      payload.probe = probe.value && !probe.value.cached ? probe.value : null
    }
    if (isEdit.value) {
      await adminClient.updateServer(props.server.id, payload)
      if (form.value.baseLoadW > 0 && props.server.power?.rapl) {
        await adminClient.recalibratePower(props.server.id, Number(form.value.baseLoadW))
      }
    } else {
      const { id } = await adminClient.createServer(payload)
      if (form.value.baseLoadW > 0 && probe.value?.profile?.power?.rapl) {
        await adminClient.recalibratePower(id, Number(form.value.baseLoadW))
      }
    }
    clearDraft()
    emit('saved')
  } catch (e) {
    saveError.value = e?.message || '保存失败'
  } finally {
    saving.value = false
  }
}

function onKey(e) {
  if (e.key === 'Escape') emit('close')
}

function resetForm() {
  const keep = form.value.authType
  form.value = { ...draftForm(), authType: keep }
  probe.value = null
  probeError.value = ''
  saveError.value = ''
  clearDraft()
}

onMounted(() => {
  window.addEventListener('keydown', onKey)
  firstInput.value?.focus()
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div class="bt-modal-mask" @click.self="emit('close')">
    <div
      class="bt-modal bt-modal--lg"
      role="dialog"
      aria-modal="true"
      aria-labelledby="server-editor-title"
    >
      <div class="bt-modal__head">
        <div id="server-editor-title" class="bt-modal__title">
          {{ isEdit ? `编辑节点 · ${server.name}` : '添加节点' }}
        </div>
        <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="emit('close')">关闭</button>
      </div>

      <div class="bt-modal__body">
        <div v-if="isSelf" class="bt-alert bt-alert--info" style="margin-bottom: 16px" role="note">
          <AppIcon name="check" aria-hidden="true" />
          本机节点：面板所在服务器，通过本地进程自动采集，无需 SSH 凭据，不可删除。
        </div>
        <Transition name="page-sub">
          <div v-if="restoredFromDraft" class="bt-alert bt-alert--info" style="margin-bottom: 16px" role="status">
            <AppIcon name="check" aria-hidden="true" />
            <span style="flex: 1">已恢复上次未保存的填写内容（密码/私钥出于安全不予缓存，需重新输入）</span>
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="resetForm">清空重填</button>
          </div>
        </Transition>

        <section style="margin-bottom: 20px">
          <h4 class="bt-card__title" style="margin-bottom: 12px">基本信息</h4>
          <div class="bt-form-grid">
            <label class="bt-field">
              <span class="bt-field__label">节点名称<i class="req">*</i></span>
              <input ref="firstInput" v-model="form.name" class="bt-input" type="text" placeholder="如：香港 · 轻量 01" />
            </label>
            <label class="bt-field">
              <span class="bt-field__label">地区（留空 = 按公网 IP 自动定位）</span>
              <input v-model="form.region" class="bt-input" type="text" placeholder="香港" />
            </label>
            <label class="bt-field span-2">
              <span class="bt-field__label">标签（逗号分隔）</span>
              <input v-model="form.tagsText" class="bt-input" type="text" placeholder="Web, 主力" />
            </label>
            <label class="bt-field">
              <span class="bt-field__label">公开备注（访客可见）</span>
              <input v-model="form.note_public" class="bt-input" type="text" placeholder="可留空" />
            </label>
            <label class="bt-field">
              <span class="bt-field__label">私有备注（仅管理端）</span>
              <input v-model="form.note_private" class="bt-input" type="text" placeholder="用途 / 账单 / 到期日" />
            </label>
            <label class="bt-check span-2">
              <input v-model="form.hidden" type="checkbox" />
              <span>在公开页隐藏此节点（仅管理端可见）</span>
            </label>
          </div>
        </section>

        <section v-if="!isSelf" style="margin-bottom: 20px">
          <h4 class="bt-card__title" style="margin-bottom: 4px">SSH 连接</h4>
          <p class="bt-text-muted" style="font-size: 12px; margin-bottom: 12px">
            凭据加密存储，读取接口永不回显；编辑时留空 = 保留原值
          </p>
          <div class="bt-form-grid">
            <label class="bt-field">
              <span class="bt-field__label">地址<i class="req">*</i></span>
              <input v-model="form.host" class="bt-input mono" type="text" placeholder="IP 或域名" />
            </label>
            <label class="bt-field">
              <span class="bt-field__label">端口</span>
              <input v-model="form.port" class="bt-input" type="number" min="1" max="65535" />
            </label>
            <label class="bt-field">
              <span class="bt-field__label">用户名<i class="req">*</i></span>
              <input v-model="form.sshUsername" class="bt-input" type="text" placeholder="root" />
            </label>
            <div class="bt-field">
              <span class="bt-field__label" id="auth-type-label">认证方式</span>
              <div class="bt-seg" role="group" aria-labelledby="auth-type-label">
                <button type="button" class="bt-seg__item" :aria-pressed="form.authType === 'password'" @click="form.authType = 'password'">密码</button>
                <button type="button" class="bt-seg__item" :aria-pressed="form.authType === 'key'" @click="form.authType = 'key'">私钥</button>
              </div>
            </div>
            <label v-if="form.authType === 'password'" class="bt-field span-2">
              <span class="bt-field__label">{{ isEdit ? '密码（留空保留原值）' : '密码' }}</span>
              <input v-model="form.password" class="bt-input" type="password" autocomplete="new-password" placeholder="••••••••" />
            </label>
            <template v-else>
              <label class="bt-field span-2">
                <span class="bt-field__label">{{ isEdit ? '私钥（留空保留原值）' : '私钥（OpenSSH 格式）' }}</span>
                <textarea v-model="form.privateKey" class="bt-textarea mono" rows="3" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" />
              </label>
              <label class="bt-field">
                <span class="bt-field__label">私钥口令（可空）</span>
                <input v-model="form.passphrase" class="bt-input" type="password" autocomplete="off" />
              </label>
            </template>
          </div>

          <div v-if="isEdit && probe?.cached && connChanged" class="bt-alert bt-alert--info" style="margin-top: 12px" role="status">
            <AppIcon name="warn" aria-hidden="true" />
            连接信息已修改，下方为旧画像 —— 建议重新试连以刷新画像后再保存。
          </div>

          <div style="display: flex; align-items: center; gap: 12px; margin-top: 12px; flex-wrap: wrap">
            <button class="bt-btn bt-btn--default" type="button" :disabled="probing || !form.host || !form.sshUsername" @click="runProbe">
              <AppIcon name="refresh" :class="{ 'is-spin': probing }" aria-hidden="true" />
              {{ probing ? '正在试连…' : '测试连接并回读画像' }}
            </button>
            <Transition name="page-sub">
              <span v-if="probeError" class="bt-text-danger" style="font-size: 12px" role="alert">
                {{ probeError }}
              </span>
            </Transition>
          </div>

          <Transition name="page-sub">
            <div v-if="probe" class="bt-alert" :class="probe.cached ? 'bt-alert--info' : 'bt-alert--success'" style="margin-top: 12px; display: block">
              <div style="display: flex; align-items: center; gap: 6px; font-weight: 600; margin-bottom: 8px">
                <AppIcon name="check" aria-hidden="true" />
                <span v-if="probe.cached">已保存的画像（修改连接信息后重新试连可刷新）</span>
                <span v-else>连接成功 · 延迟 {{ probe.latency_ms }}ms · 保存时将自动带入本次试连画像/指纹/地区</span>
              </div>
              <dl class="bt-def-grid" style="padding-top: 0">
                <div class="bt-def"><dt>主机名</dt><dd class="mono">{{ probe.profile.hostname }}</dd></div>
                <div class="bt-def"><dt>系统</dt><dd>{{ probe.profile.os_name }} {{ probe.profile.os_version }} · {{ probe.profile.arch }}</dd></div>
                <div class="bt-def"><dt>CPU</dt><dd>{{ probe.profile.cpu_model }} · {{ probe.profile.cpu_cores }}核</dd></div>
                <div class="bt-def"><dt>内存 / 磁盘</dt><dd>{{ (probe.profile.mem_total / 1024 ** 3).toFixed(0) }}G / {{ (probe.profile.disk_total / 1024 ** 3).toFixed(0) }}G</dd></div>
                <div class="bt-def"><dt>虚拟化</dt><dd>{{ probe.profile.virt }}</dd></div>
                <div class="bt-def"><dt>公网 IP（私有）</dt><dd class="mono">{{ probe.geo?.public_ip || '—' }}{{ probe.geo ? ` · ${probe.geo.country}` : '' }}</dd></div>
                <div class="bt-def"><dt>Host Key 指纹</dt><dd class="mono">{{ probe.host_key_fp || '—' }}</dd></div>
                <div class="bt-def">
                  <dt>功耗能力</dt>
                  <dd :class="probe.profile.power?.rapl ? 'bt-text-ok' : 'bt-text-muted'">
                    {{ probe.profile.power?.rapl ? `RAPL 可用 · 基础功耗 ${fmtWatts(probe.profile.power.base_load_w)}` : '未暴露 RAPL 功率计（AMD/虚拟机常见）' }}
                    {{ probe.profile.power?.battery ? ' · 含电池（可自动校准）' : '' }}
                  </dd>
                </div>
              </dl>
            </div>
          </Transition>
        </section>

        <section v-if="probe?.profile?.power?.rapl">
          <h4 class="bt-card__title" style="margin-bottom: 4px">功耗校准</h4>
          <p class="bt-text-muted" style="font-size: 12px; margin-bottom: 12px">
            整机功耗 = CPU + 内存 + 基础功耗；有电池的机器放电时自动校准，也可在此手动设定
          </p>
          <div class="bt-form-grid">
            <label class="bt-field">
              <span class="bt-field__label">基础功耗（W）</span>
              <input v-model.number="form.baseLoadW" class="bt-input" type="number" min="0" max="200" step="0.1" />
            </label>
            <div class="bt-field">
              <span class="bt-field__label">当前来源</span>
              <div style="font-size: 13px; color: var(--bt-text-2); padding: 7px 0">
                {{ probe.profile.power.base_load_source === 'calibration' ? '电池自动校准' : '默认估算' }}
              </div>
            </div>
          </div>
        </section>

        <Transition name="page-sub">
          <div v-if="saveError" class="bt-alert bt-alert--error" style="margin-top: 12px" role="alert">
            <AppIcon name="warn" aria-hidden="true" />{{ saveError }}
          </div>
        </Transition>
      </div>

      <div class="bt-modal__foot">
        <button class="bt-btn bt-btn--ghost" type="button" @click="emit('close')">取消</button>
        <button class="bt-btn bt-btn--primary" type="button" :disabled="saving" @click="save">
          <AppIcon v-if="saving" name="refresh" class="is-spin" aria-hidden="true" />
          {{ saving ? '保存中…' : isEdit ? '保存修改' : '保存并添加' }}
        </button>
      </div>
    </div>
  </div>
</template>
