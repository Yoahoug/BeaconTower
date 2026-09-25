<script setup>
// 安全与账号：修改密码（需旧密码）+ 数据管理（原型：重置本地演示数据）。
import { ref } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { adminApi, passwordStrength } from './store'

const oldPw = ref('')
const newPw = ref('')
const confirmPw = ref('')
const error = ref('')
const ok = ref(false)
const busy = ref(false)

const strength = ref({ ok: false })

function onNewPw() {
  strength.value = passwordStrength(newPw.value)
  ok.value = false
}

async function submit() {
  error.value = ''
  if (newPw.value !== confirmPw.value) return (error.value = '两次输入的新密码不一致')
  busy.value = true
  try {
    await adminApi.changePassword({ old_password: oldPw.value, new_password: newPw.value })
    ok.value = true
    oldPw.value = newPw.value = confirmPw.value = ''
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

async function resetDemo() {
  if (!confirm('重置将清空本浏览器中的演示数据（管理员账号、节点、设置与审计），确定？')) return
  await adminApi.resetAll()
  location.reload()
}
</script>

<template>
  <div class="settings-admin">
    <div class="glass-card settings-card">
      <h4 class="sec-title">修改管理员密码</h4>
      <form class="form-grid" @submit.prevent="submit">
        <label class="field">
          <span class="field-label">旧密码</span>
          <input v-model="oldPw" type="password" autocomplete="current-password" required />
        </label>
        <label class="field">
          <span class="field-label">新密码</span>
          <input v-model="newPw" type="password" autocomplete="new-password" required @input="onNewPw" />
        </label>
        <label class="field">
          <span class="field-label">确认新密码</span>
          <input v-model="confirmPw" type="password" autocomplete="new-password" required />
        </label>
        <div class="field-wide pw-rules">
          <span class="pw-rule" :class="{ ok: strength.length }">至少 10 位</span>
          <span class="pw-rule" :class="{ ok: strength.lower }">小写字母</span>
          <span class="pw-rule" :class="{ ok: strength.upper }">大写字母</span>
          <span class="pw-rule" :class="{ ok: strength.digit }">数字</span>
          <span class="pw-rule" :class="{ ok: strength.symbol }">符号</span>
        </div>
        <div v-if="error" class="form-error field-wide"><AppIcon name="warn" class="err-ico" />{{ error }}</div>
        <div v-if="ok" class="form-ok field-wide"><AppIcon name="check" class="ok-ico" />密码已更新</div>
        <div class="field-wide">
          <button class="btn btn-primary" type="submit" :disabled="busy">{{ busy ? '提交中…' : '更新密码' }}</button>
        </div>
      </form>
    </div>

    <div class="glass-card settings-card">
      <h4 class="sec-title">安全机制</h4>
      <ul class="mech-list">
        <li><AppIcon name="shield" class="mech-ico" />SSH 凭据 AES-256-GCM 加密存储，主密钥独立于数据库（后端实现项）</li>
        <li><AppIcon name="shield" class="mech-ico" />密码 Argon2id 哈希；会话 Cookie HttpOnly + SameSite=Strict + CSRF 双提交</li>
        <li><AppIcon name="shield" class="mech-ico" />登录失败递增封禁：5 分钟 → 15 分钟 → 1 小时 → 24 小时</li>
        <li><AppIcon name="shield" class="mech-ico" />采集命令内置白名单，架构上不提供任何远程命令执行入口</li>
      </ul>
    </div>

    <div class="glass-card settings-card">
      <h4 class="sec-title">原型数据</h4>
      <p class="sec-hint">当前为前端原型：管理数据保存在浏览器 localStorage，用于演示完整交互流程。后端接入后迁移为 SQLite + 加密凭据。</p>
      <button class="btn btn-danger-ghost" @click="resetDemo">重置演示数据</button>
    </div>
  </div>
</template>
