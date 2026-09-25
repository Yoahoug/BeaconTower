<script setup>
// 初始化向导 + 登录：未初始化时展示两步向导（账号 → 确认），已初始化时展示登录。
// 密码强度按 05 文档策略实时校验（≥10 位，大小写+数字+符号）。
import { ref, computed, onMounted } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { adminApi, passwordStrength } from './store'

const emit = defineEmits(['authed'])

const initialized = ref(true)
const mode = ref('login') // login | setup | setup-confirm
const username = ref('')
const password = ref('')
const password2 = ref('')
const error = ref('')
const busy = ref(false)

const strength = computed(() => passwordStrength(password.value))
const strengthItems = computed(() => [
  { key: 'length', label: '至少 10 位', ok: strength.value.length },
  { key: 'lower', label: '小写字母', ok: strength.value.lower },
  { key: 'upper', label: '大写字母', ok: strength.value.upper },
  { key: 'digit', label: '数字', ok: strength.value.digit },
  { key: 'symbol', label: '符号', ok: strength.value.symbol },
])

onMounted(async () => {
  const st = await adminApi.status()
  initialized.value = st.initialized
  mode.value = st.initialized ? 'login' : 'setup'
})

async function submit() {
  error.value = ''
  busy.value = true
  try {
    if (mode.value === 'login') {
      await adminApi.login({ username: username.value, password: password.value })
      emit('authed')
    } else if (mode.value === 'setup') {
      if (!strength.value.ok) throw new Error('密码不满足强度要求')
      mode.value = 'setup-confirm'
    } else {
      if (password.value !== password2.value) throw new Error('两次输入的密码不一致')
      await adminApi.setup({ username: username.value, password: password.value })
      emit('authed')
    }
  } catch (e) {
    error.value = e.message
    if (mode.value === 'setup-confirm') mode.value = 'setup'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="auth-wrap">
    <div class="glass-card auth-card">
      <div class="auth-brand">
        <span class="auth-logo"><AppIcon name="beacon" /></span>
        <div>
          <h2>{{ mode === 'login' ? '登录管理面板' : '初始化向导' }}</h2>
          <p class="auth-sub">
            {{ mode === 'login' ? '仅管理员可访问，此页面不设公开入口' : mode === 'setup' ? '首次使用：创建管理员账号（仅需一次）' : '请再次输入密码确认' }}
          </p>
        </div>
      </div>

      <form class="auth-form" @submit.prevent="submit">
        <label class="field">
          <span class="field-label">用户名</span>
          <input v-model="username" type="text" autocomplete="username" placeholder="admin" required />
        </label>

        <label class="field">
          <span class="field-label">密码</span>
          <input
            v-model="password"
            type="password"
            :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
            placeholder="••••••••••"
            required
          />
        </label>

        <!-- 强度清单（仅初始化时显示） -->
        <div v-if="mode !== 'login'" class="pw-rules">
          <span v-for="r in strengthItems" :key="r.key" class="pw-rule" :class="{ ok: r.ok }">
            <AppIcon :name="r.ok ? 'check' : 'plus'" class="pw-rule-ico" />{{ r.label }}
          </span>
        </div>

        <label v-if="mode === 'setup-confirm'" class="field">
          <span class="field-label">确认密码</span>
          <input v-model="password2" type="password" autocomplete="new-password" placeholder="再次输入" required />
        </label>

        <div v-if="error" class="form-error"><AppIcon name="warn" class="err-ico" />{{ error }}</div>

        <button class="btn btn-primary btn-block" type="submit" :disabled="busy">
          {{ busy ? '请稍候…' : mode === 'login' ? '登录' : mode === 'setup' ? '下一步' : '完成初始化' }}
        </button>
      </form>

      <p class="auth-foot">
        密码以 Argon2id 哈希存储 · SSH 凭据 AES-256-GCM 加密 · 登录失败将限速封禁
      </p>
    </div>
  </div>
</template>
