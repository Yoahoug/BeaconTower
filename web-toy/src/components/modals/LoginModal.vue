<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs">
    <div
      class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-6 relative overflow-hidden"
      :style="{ boxShadow: '6px 6px 0px #121212' }"
      role="dialog"
      aria-modal="true"
      aria-label="管理员登录"
    >
      <!-- 弹窗顶栏 -->
      <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-lg bg-toy-yellow border-2 border-toy-black flex items-center justify-center">
            <ToyMenuIcons name="security" class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-base font-black text-toy-black">{{ setupMode ? '初始化管理员' : '管理员登录' }}</h3>
            <p class="text-[10px] font-bold text-toy-muted">
              {{ setupMode ? '首次使用：创建管理员账号后即可纳管节点' : '安全凭证验证' }}
            </p>
          </div>
        </div>
        <button
          @click="$emit('close')"
          class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
          aria-label="关闭"
        >
          ✕
        </button>
      </div>

      <!-- 表单内容 -->
      <form @submit.prevent="handleSubmit" class="mt-4 space-y-4">
        <div>
          <label class="block text-xs font-black text-toy-black mb-1">用户名</label>
          <input
            v-model="username"
            type="text"
            :placeholder="setupMode ? '3–32 位字母/数字/下划线/点/横线' : '管理员用户名'"
            autocomplete="username"
            class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none focus:ring-2 focus:ring-toy-yellow text-toy-black shadow-toy-sm"
            required
          />
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">密码</label>
          <input
            v-model="password"
            type="password"
            :placeholder="setupMode ? '至少 10 位，建议大小写+数字+符号' : '请输入管理员密码...'"
            :autocomplete="setupMode ? 'new-password' : 'current-password'"
            class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none focus:ring-2 focus:ring-toy-yellow text-toy-black shadow-toy-sm"
            required
          />
        </div>

        <!-- 初始化：确认密码 + 可选初始化令牌 -->
        <template v-if="setupMode">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">确认密码</label>
            <input
              v-model="confirmPassword"
              type="password"
              placeholder="再次输入同一密码"
              autocomplete="new-password"
              class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none focus:ring-2 focus:ring-toy-yellow text-toy-black shadow-toy-sm"
              required
            />
          </div>
          <!-- 强度清单：与后端 passwordOK 同规则（≥10 位 + 大小写 + 数字 + 符号） -->
          <div class="p-2.5 bg-toy-bg border-2 border-dashed border-toy-black rounded-xl">
            <div class="text-[10px] font-black text-toy-muted mb-1.5">密码强度要求</div>
            <ul class="grid grid-cols-2 gap-x-2 gap-y-1">
              <li
                v-for="item in strengthItems"
                :key="item.key"
                class="flex items-center gap-1 text-[11px] font-bold"
                :class="item.ok ? 'text-emerald-600' : 'text-toy-muted'"
              >
                <span class="w-3.5 h-3.5 rounded-full border-2 border-current flex items-center justify-center text-[8px] font-black">
                  {{ item.ok ? '✓' : '' }}
                </span>
                {{ item.label }}
              </li>
            </ul>
          </div>
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">
              初始化令牌<span class="text-toy-muted font-bold">（部署启用了 SETUP_TOKEN 时必填）</span>
            </label>
            <input
              v-model="setupToken"
              type="text"
              placeholder="留空表示未启用"
              class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none focus:ring-2 focus:ring-toy-yellow text-toy-black shadow-toy-sm font-mono"
            />
          </div>
        </template>

        <div v-if="errorMsg" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700" role="alert">
          {{ errorMsg }}
        </div>

        <div class="pt-2 flex items-center justify-end gap-2">
          <button
            type="button"
            @click="$emit('close')"
            class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer"
          >
            取消
          </button>
          <button
            type="submit"
            :disabled="busy"
            class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-60"
          >
            {{ busy ? (setupMode ? '初始化中…' : '验证中…') : (setupMode ? '创建管理员并进入' : '登录进入控制台') }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { api } from '../../services/api';
import ToyMenuIcons from '../icons/ToyMenuIcons.vue';

const props = defineProps<{
  isOpen: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'success'): void;
}>();

const username = ref('');
const password = ref('');
const confirmPassword = ref('');
const setupToken = ref('');
const errorMsg = ref('');
const busy = ref(false);
// 未初始化的面板：登录框自动切换为「首次初始化」表单（后端 POST /admin/setup）
const setupMode = ref(false);

// 与后端 usernameRe / passwordOK 完全同规则，错误在提交前就暴露
const USERNAME_RE = /^[A-Za-z0-9_.-]{3,32}$/;
const strengthItems = computed(() => [
  { key: 'length', label: '至少 10 位', ok: password.value.length >= 10 },
  { key: 'lower', label: '小写字母', ok: /[a-z]/.test(password.value) },
  { key: 'upper', label: '大写字母', ok: /[A-Z]/.test(password.value) },
  { key: 'digit', label: '数字', ok: /\d/.test(password.value) },
  { key: 'symbol', label: '符号', ok: /[^A-Za-z0-9]/.test(password.value) },
]);

watch(() => props.isOpen, async (open) => {
  if (!open) return;
  errorMsg.value = '';
  password.value = '';
  confirmPassword.value = '';
  try {
    const st = await api.getAdminStatus();
    setupMode.value = !st.initialized;
  } catch {
    setupMode.value = false;
  }
});

async function handleSubmit() {
  errorMsg.value = '';
  busy.value = true;
  try {
    if (setupMode.value) {
      const u = username.value.trim();
      if (!USERNAME_RE.test(u)) {
        errorMsg.value = '用户名须为 3–32 位字母/数字/下划线/点/横线';
        return;
      }
      if (!strengthItems.value.every((i) => i.ok)) {
        errorMsg.value = '密码至少 10 位，须含大小写字母、数字与符号';
        return;
      }
      if (password.value !== confirmPassword.value) {
        errorMsg.value = '两次输入的密码不一致';
        return;
      }
      await api.setup({
        username: u,
        password: password.value,
        setup_token: setupToken.value.trim() || undefined,
      });
      password.value = '';
      confirmPassword.value = '';
      emit('success');
      emit('close');
      return;
    }

    // 真实登录：POST /admin/login，会话 cookie + CSRF 由 http.ts 统一处理
    const res = await api.login(username.value.trim(), password.value);
    if (res.success) {
      password.value = '';
      emit('success');
      emit('close');
    } else {
      errorMsg.value = res.msg || '登录失败';
    }
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : (setupMode.value ? '初始化失败' : '登录失败');
  } finally {
    busy.value = false;
  }
}
</script>
