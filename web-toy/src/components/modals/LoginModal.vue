<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs">
    <div 
      class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-6 relative overflow-hidden"
      :style="{ boxShadow: '6px 6px 0px #121212' }"
    >
      <!-- 弹窗顶栏 -->
      <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-lg bg-toy-yellow border-2 border-toy-black flex items-center justify-center">
            <ToyMenuIcons name="security" class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-base font-black text-toy-black">管理员登录</h3>
            <p class="text-[10px] font-bold text-toy-muted">安全凭证验证</p>
          </div>
        </div>
        <button 
          @click="$emit('close')"
          class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
        >
          ✕
        </button>
      </div>

      <!-- 表单内容 -->
      <form @submit.prevent="handleSubmit" class="mt-4 space-y-4">
        <div>
          <label class="block text-xs font-black text-toy-black mb-1">主管理员密码</label>
          <input 
            v-model="password"
            type="password" 
            placeholder="请输入管理员密码..." 
            class="w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none focus:ring-2 focus:ring-toy-yellow text-toy-black shadow-toy-sm"
            required
            autofocus
          />
        </div>

        <div v-if="errorMsg" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">
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
            class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer"
          >
            登录进入控制台
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { api } from '../../services/api';
import ToyMenuIcons from '../icons/ToyMenuIcons.vue';

defineProps<{
  isOpen: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'success'): void;
}>();

const password = ref('');
const errorMsg = ref('');

async function handleSubmit() {
  errorMsg.value = '';
  const res = await api.login(password.value);
  if (res.success) {
    password.value = '';
    emit('success');
    emit('close');
  } else {
    errorMsg.value = res.msg || '登录失败';
  }
}
</script>
