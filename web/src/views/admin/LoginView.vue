<!-- 登录页：极光背景上的玻璃卡 + 信标徽标呼吸光环 -->
<script setup>
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import AppIcon from '../../components/AppIcon.vue'
import { authClient } from '../../api/auth'
import { useAdminStore } from '../../stores/admin'
import { useUiStore } from '../../stores/ui'

const router = useRouter()
const route = useRoute()
const admin = useAdminStore()
const ui = useUiStore()

const username = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await authClient.login({ username: username.value, password: password.value })
    admin.markAuthed(username.value)
    ui.notify('登录成功')
    const rawRedirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/admin/servers'
    const redirect = rawRedirect.startsWith('/admin') ? rawRedirect : '/admin/servers'
    router.push(redirect)
  } catch (e) {
    error.value = e?.message || '登录失败'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="auth-wrap">
    <div class="bt-card spot auth-card bt-enter" v-spotlight style="--i: 0">
      <div class="auth-brand">
        <span class="auth-logo bt-beacon-glow" aria-hidden="true"><AppIcon name="beacon" /></span>
        <div>
          <h2>登录管理面板</h2>
          <p class="auth-sub">仅管理员可访问，此页面不设公开入口</p>
        </div>
      </div>

      <form class="auth-form" @submit.prevent="submit">
        <label class="bt-field">
          <span class="bt-field__label">用户名</span>
          <input v-model="username" class="bt-input" type="text" autocomplete="username" placeholder="admin" required />
        </label>

        <label class="bt-field">
          <span class="bt-field__label">密码</span>
          <input
            v-model="password"
            class="bt-input"
            type="password"
            autocomplete="current-password"
            placeholder="••••••••••"
            required
          />
        </label>

        <Transition name="page-sub">
          <div v-if="error" class="bt-alert bt-alert--error" role="alert">
            <AppIcon name="warn" aria-hidden="true" />{{ error }}
          </div>
        </Transition>

        <button class="bt-btn bt-btn--primary bt-btn--block" type="submit" :disabled="busy">
          <AppIcon v-if="busy" name="refresh" class="is-spin" aria-hidden="true" />
          {{ busy ? '请稍候…' : '登录' }}
        </button>
      </form>

      <p class="auth-foot">密码以 Argon2id 哈希存储 · SSH 凭据 AES-256-GCM 加密 · 登录失败将限速封禁</p>
    </div>
  </div>
</template>
