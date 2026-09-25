<!-- 安全与账号：改密 + 安全机制说明 + 演示数据重置（v2.0） -->
<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../../components/AppIcon.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import { authClient, passwordStrength } from '../../api/auth'
import { adminClient } from '../../api/admin'
import { useAdminStore } from '../../stores/admin'
import { useUiStore } from '../../stores/ui'

const router = useRouter()
const admin = useAdminStore()
const ui = useUiStore()

const oldPw = ref('')
const newPw = ref('')
const confirmPw = ref('')
const error = ref('')
const ok = ref(false)
const busy = ref(false)
const strength = ref({ ok: false })
const showReset = ref(false)
const resetting = ref(false)

function onNewPw() {
  strength.value = passwordStrength(newPw.value)
  ok.value = false
}

async function submit() {
  error.value = ''
  if (newPw.value !== confirmPw.value) {
    error.value = '两次输入的新密码不一致'
    return
  }
  busy.value = true
  try {
    await authClient.changePassword({ old_password: oldPw.value, new_password: newPw.value })
    ok.value = true
    oldPw.value = newPw.value = confirmPw.value = ''
    ui.notify('密码已更新')
  } catch (e) {
    error.value = e?.message || '修改失败'
  } finally {
    busy.value = false
  }
}

async function resetDemo() {
  resetting.value = true
  try {
    await adminClient.resetAll()
    showReset.value = false
    await admin.logout()
    ui.notify('演示数据已重置')
    router.push('/admin/setup')
  } finally {
    resetting.value = false
  }
}
</script>

<template>
  <div class="settings-stack">
    <section class="bt-card bt-enter" style="--i: 0" aria-labelledby="sec-pw">
      <div class="bt-card__head"><div id="sec-pw" class="bt-card__title">修改管理员密码</div></div>
      <div class="bt-card__body">
        <form class="bt-form-grid" @submit.prevent="submit">
          <label class="bt-field">
            <span class="bt-field__label">旧密码</span>
            <input v-model="oldPw" class="bt-input" type="password" autocomplete="current-password" required />
          </label>
          <label class="bt-field">
            <span class="bt-field__label">新密码</span>
            <input v-model="newPw" class="bt-input" type="password" autocomplete="new-password" required @input="onNewPw" />
          </label>
          <label class="bt-field">
            <span class="bt-field__label">确认新密码</span>
            <input v-model="confirmPw" class="bt-input" type="password" autocomplete="new-password" required />
          </label>
          <div class="span-2 bt-pw-rules" aria-label="新密码强度">
            <span class="bt-pw-rule" :class="{ 'is-ok': strength.length }">至少 10 位</span>
            <span class="bt-pw-rule" :class="{ 'is-ok': strength.lower }">小写字母</span>
            <span class="bt-pw-rule" :class="{ 'is-ok': strength.upper }">大写字母</span>
            <span class="bt-pw-rule" :class="{ 'is-ok': strength.digit }">数字</span>
            <span class="bt-pw-rule" :class="{ 'is-ok': strength.symbol }">符号</span>
          </div>
          <Transition name="page-sub">
            <div v-if="error" class="span-2 bt-alert bt-alert--error" role="alert">
              <AppIcon name="warn" aria-hidden="true" />{{ error }}
            </div>
          </Transition>
          <Transition name="page-sub">
            <div v-if="ok" class="span-2 bt-alert bt-alert--success" role="status">
              <AppIcon name="check" aria-hidden="true" />密码已更新
            </div>
          </Transition>
          <div class="span-2">
            <button class="bt-btn bt-btn--primary" type="submit" :disabled="busy">
              <AppIcon v-if="busy" name="refresh" class="is-spin" aria-hidden="true" />
              {{ busy ? '提交中…' : '更新密码' }}
            </button>
          </div>
        </form>
      </div>
    </section>

    <section class="bt-card bt-enter" style="--i: 1" aria-labelledby="sec-mech">
      <div class="bt-card__head"><div id="sec-mech" class="bt-card__title">安全机制</div></div>
      <div class="bt-card__body">
        <ul class="bt-mech-list">
          <li><AppIcon name="shield" aria-hidden="true" />SSH 凭据 AES-256-GCM 加密存储，主密钥独立于数据库（后端实现项）</li>
          <li><AppIcon name="shield" aria-hidden="true" />密码 Argon2id 哈希；会话 Cookie HttpOnly + SameSite=Strict + CSRF 双提交</li>
          <li><AppIcon name="shield" aria-hidden="true" />登录失败递增封禁：5 分钟 → 15 分钟 → 1 小时 → 24 小时</li>
          <li><AppIcon name="shield" aria-hidden="true" />采集命令内置白名单，架构上不提供任何远程命令执行入口</li>
        </ul>
      </div>
    </section>

    <section class="bt-card bt-enter" style="--i: 2" aria-labelledby="sec-demo">
      <div class="bt-card__head"><div id="sec-demo" class="bt-card__title">原型数据</div></div>
      <div class="bt-card__body">
        <p class="bt-text-muted" style="font-size: 13px; margin-bottom: 12px">
          当前为前端原型：管理数据保存在浏览器 localStorage，用于演示完整交互流程。后端接入后迁移为 SQLite + 加密凭据。
        </p>
        <button class="bt-btn bt-btn--danger-ghost" type="button" @click="showReset = true">重置演示数据</button>
      </div>
    </section>

    <Transition name="modal">
      <ConfirmDialog
        v-if="showReset"
        title="重置演示数据"
        message="将清空本浏览器中的演示数据（管理员账号、节点、设置与审计），并返回初始化向导。确定？"
        confirm-text="确认重置"
        :busy="resetting"
        danger
        @cancel="showReset = false"
        @confirm="resetDemo"
      />
    </Transition>
  </div>
</template>
