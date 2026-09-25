<!-- 初始化向导：两步（账号 → 确认），完成后进入节点管理 -->
<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../../components/AppIcon.vue'
import { adminClient, passwordStrength } from '../../api/admin'
import { useAdminStore } from '../../stores/admin'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'

const router = useRouter()
const admin = useAdminStore()

const step = ref('form') // form | confirm
const username = ref('')
const password = ref('')
const password2 = ref('')
const error = ref('')
const busy = ref(false)
const checking = ref(true)

const strength = computed(() => passwordStrength(password.value))
const strengthItems = computed(() => [
  { key: 'length', label: '至少 10 位', ok: strength.value.length },
  { key: 'lower', label: '小写字母', ok: strength.value.lower },
  { key: 'upper', label: '大写字母', ok: strength.value.upper },
  { key: 'digit', label: '数字', ok: strength.value.digit },
  { key: 'symbol', label: '符号', ok: strength.value.symbol },
])

onMounted(async () => {
  try {
    const st = await adminClient.status()
    admin.initialized = st.initialized
    if (st.initialized) {
      router.replace(st.loggedIn ? '/admin/servers' : '/admin/login')
      return
    }
  } finally {
    checking.value = false
  }
})

async function submit() {
  error.value = ''
  busy.value = true
  try {
    if (step.value === 'form') {
      if (!username.value || username.value.length < 3) throw new Error('用户名至少 3 个字符')
      if (!strength.value.ok) throw new Error('密码不满足强度要求')
      step.value = 'confirm'
    } else {
      if (password.value !== password2.value) throw new Error('两次输入的密码不一致')
      await adminClient.setup({ username: username.value, password: password.value })
      admin.markAuthed(username.value)
      router.push('/admin/servers')
    }
  } catch (e) {
    error.value = e?.message || '初始化失败'
    if (step.value === 'confirm' && error.value === '两次输入的密码不一致') step.value = 'form'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="auth-wrap">
    <div v-if="checking" class="bt-card auth-card">
      <StateSkeleton :rows="3" />
    </div>
    <div v-else class="bt-card auth-card">
      <div class="auth-brand">
        <span class="auth-logo" aria-hidden="true"><AppIcon name="beacon" /></span>
        <div>
          <h2>初始化向导</h2>
          <p class="auth-sub">{{ step === 'form' ? '首次使用：创建管理员账号（仅需一次）' : '请再次输入密码确认' }}</p>
        </div>
      </div>

      <form class="auth-form" @submit.prevent="submit">
        <label class="bt-field">
          <span class="bt-field__label">用户名<i class="req">*</i></span>
          <input v-model="username" class="bt-input" type="text" autocomplete="username" placeholder="admin" required />
        </label>

        <label class="bt-field">
          <span class="bt-field__label">密码<i class="req">*</i></span>
          <input v-model="password" class="bt-input" type="password" autocomplete="new-password" placeholder="••••••••••" required />
        </label>

        <div class="bt-pw-rules" aria-label="密码强度要求">
          <span v-for="r in strengthItems" :key="r.key" class="bt-pw-rule" :class="{ 'is-ok': r.ok }">
            <AppIcon :name="r.ok ? 'check' : 'plus'" aria-hidden="true" />{{ r.label }}
          </span>
        </div>

        <label v-if="step === 'confirm'" class="bt-field">
          <span class="bt-field__label">确认密码<i class="req">*</i></span>
          <input v-model="password2" class="bt-input" type="password" autocomplete="new-password" placeholder="再次输入" required />
        </label>

        <div v-if="error" class="bt-alert bt-alert--error" role="alert">
          <AppIcon name="warn" aria-hidden="true" />{{ error }}
        </div>

        <button class="bt-btn bt-btn--primary bt-btn--block" type="submit" :disabled="busy">
          {{ busy ? '请稍候…' : step === 'form' ? '下一步' : '完成初始化' }}
        </button>
      </form>
    </div>
  </div>
</template>
