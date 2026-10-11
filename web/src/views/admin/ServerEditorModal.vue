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
    // probe 剥离 panel_private_key：面板代管私钥只存在于当次试连响应内存里，
    // 绝不入草稿缓存（恢复草稿后需重新试连装机才能保存）
    const { panel_private_key: _ppk, ...safeProbe } = probe.value || {}
    sessionStorage.setItem(
      DRAFT_KEY,
      JSON.stringify({ form: safe, probe: probe.value ? safeProbe : null }),
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
    // 本机节点的占位凭据 host=local 不当作真实地址；尚无地址时预填后端探测到的
    // 容器默认网关（容器可达的宿主地址），用户可改
    host:
      props.server?.ssh?.host === 'local'
        ? props.server?.ssh_suggest_host || ''
        : props.server?.ssh?.host || props.server?.ssh_suggest_host || '',
    port: props.server?.ssh?.port || 22,
    sshUsername: props.server?.ssh?.username || '',
    // 认证三态：auto = 面板生成密钥（默认，试连即装钥，密码只作一次性引导不落库）；
    // password/key = 传统手工凭据。存量节点编辑时按其现有 auth_type 预填。
    authType: props.server?.ssh?.auth_type || 'auto',
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

// 密码字段标签：auto 模式下是「一次性引导密码」；手工密码模式沿用原语义
const autoBootHint = computed(() => {
  if (form.value.authType !== 'auto') return isEdit.value ? '密码（留空保留原值）' : '密码'
  return probe.value?.bootstrap ? '引导密码（已用毕，面板不会存储）' : 'SSH 密码（仅本次引导使用，不会存储）'
})

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
    const auto = form.value.authType === 'auto'
    probe.value = await adminClient.testConnection({
      host: form.value.host,
      port: form.value.port,
      username: form.value.sshUsername,
      auth_type: auto ? 'password' : form.value.authType,
      password: form.value.password || undefined,
      private_key: form.value.privateKey || undefined,
      passphrase: form.value.passphrase || undefined,
      // 试连即装钥（v2.7）：面板生成密钥装机并验证，密码只作一次性引导
      bootstrap_key: auto,
      server_id: props.server?.id || 0,
    })
    // 记录引导时的连接信息：保存时校验私钥装到的是同一台机器
    if (probe.value) {
      probe.value._bootHost = form.value.host
      probe.value._bootPort = Number(form.value.port) || 22
      probe.value._bootUser = form.value.sshUsername
    }
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
  // 本机节点允许留空（纯本地采集）；填了地址就按普通节点校验
  const sshFilled = !isSelf.value || !!form.value.host
  const auto = form.value.authType === 'auto'
  const bootKey = probe.value?.bootstrap && probe.value?.panel_private_key
  // 引导密钥只在「试连时连接信息与当前表单一致」才有效——改过地址/端口后
  // 私钥装到的是旧机器，必须重新试连
  const bootKeyFresh =
    bootKey &&
    probe.value?._bootHost === form.value.host &&
    Number(probe.value?._bootPort) === Number(form.value.port) &&
    probe.value?._bootUser === form.value.sshUsername
  if (sshFilled) {
    if (!form.value.host || !form.value.sshUsername) return (saveError.value = '请填写 SSH 地址与用户名')
    const hadKey = !!props.server?.ssh?.has_key
    if (auto && !bootKeyFresh && !hadKey) {
      return (saveError.value = '请先「测试连接」完成密钥引导（自动生成密钥需一次成功试连）')
    }
    if (auto && !bootKeyFresh && hadKey) {
      // 存量节点重选自动密钥但没重新引导：回落保留原有密钥（后端硬校验兜底）
      // ——这里不阻塞保存，但提示用户；payload 里不覆盖 panel_private_key
      return (saveError.value = '密钥引导未完成：请先点「测试连接并安装密钥」，或切换到对应认证方式后保存')
    }
    if (!auto && !hadKey && form.value.authType === 'password' && !form.value.password) {
      return (saveError.value = '请填写 SSH 密码')
    }
    if (!auto && !hadKey && form.value.authType === 'key' && !form.value.privateKey) {
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
    if (!isSelf.value || form.value.host) {
      payload.ssh = {
        host: form.value.host,
        port: Number(form.value.port) || 22,
        username: form.value.sshUsername,
        // auto 模式落库为 key（面板自管密钥）；带 panel_private_key 时后端归一
        auth_type: auto ? 'key' : form.value.authType,
        password: auto || form.value.authType !== 'password' ? undefined : form.value.password || undefined,
        private_key: auto ? undefined : form.value.privateKey || undefined,
        passphrase: auto ? undefined : form.value.passphrase || undefined,
        // 试连 bootstrap 生成的面板私钥原样带回（不经浏览器暂存）；
        // 引导过期（改过连接信息）时不得携带，后端硬校验会拒绝 key+无私钥
        panel_private_key: auto && bootKeyFresh ? probe.value.panel_private_key : undefined,
      }
      payload.probe = probe.value && !probe.value.cached ? { ...probe.value, panel_private_key: undefined } : null
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
          本机节点：面板所在服务器，采集走本地进程（不依赖 SSH），不可删除。
          若要让它加入 WG 组网，请在下方录入宿主 SSH —— 面板容器没有 NET_ADMIN，配置需要经宿主机施加。
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

        <section style="margin-bottom: 20px">
          <h4 class="bt-card__title" style="margin-bottom: 4px">
            SSH 连接<span v-if="isSelf"> · 宿主（用于本机入网）</span>
          </h4>
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
                <button type="button" class="bt-seg__item" :aria-pressed="form.authType === 'auto'" @click="form.authType = 'auto'">自动密钥</button>
                <button type="button" class="bt-seg__item" :aria-pressed="form.authType === 'password'" @click="form.authType = 'password'">密码</button>
                <button type="button" class="bt-seg__item" :aria-pressed="form.authType === 'key'" @click="form.authType = 'key'">私钥</button>
              </div>
            </div>
            <p v-if="form.authType === 'auto'" class="bt-text-muted span-2" style="font-size: 12px; margin: 0 0 4px">
              推荐。测试连接时面板自动生成专属密钥装到节点并验证，之后 SSH 全走密钥——
              下方密码仅本次引导使用，<strong>不会被面板存储</strong>。
            </p>
            <label v-if="form.authType !== 'key'" class="bt-field span-2">
              <span class="bt-field__label">{{ autoBootHint }}</span>
              <input v-model="form.password" class="bt-input" type="password" autocomplete="new-password" placeholder="••••••••" />
            </label>
            <template v-if="form.authType === 'key'">
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

          <div v-if="isSelf" class="bt-alert bt-alert--info" style="margin-top: 12px; margin-bottom: 12px" role="note">
            <AppIcon name="warn" aria-hidden="true" />
            地址要填<strong>容器能连到的</strong>宿主机地址（表单已按容器默认网关预填
            <code v-if="props.server?.ssh_suggest_host">{{ props.server.ssh_suggest_host }}</code>；也可填内网 IP）。
            不要填 WG 地址（切网/重启 WG 时面板会把自己掐断）。
          </div>

          <div v-if="isEdit && probe?.cached && connChanged" class="bt-alert bt-alert--info" style="margin-top: 12px" role="status">
            <AppIcon name="warn" aria-hidden="true" />
            连接信息已修改，下方为旧画像 —— 建议重新试连以刷新画像后再保存。
          </div>

          <div style="display: flex; align-items: center; gap: 12px; margin-top: 12px; flex-wrap: wrap">
            <button class="bt-btn bt-btn--default" type="button" :disabled="probing || !form.host || !form.sshUsername || (form.authType === 'auto' && !form.password)" @click="runProbe">
              <AppIcon name="refresh" :class="{ 'is-spin': probing }" aria-hidden="true" />
              {{ probing ? '正在连接…' : form.authType === 'auto' ? '测试连接并安装密钥' : '测试连接并回读画像' }}
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
                <span v-else-if="probe.bootstrap">密钥已装到节点并验证通过 · 延迟 {{ probe.latency_ms }}ms · 保存后面板将以密钥登录（密码不存储）</span>
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
