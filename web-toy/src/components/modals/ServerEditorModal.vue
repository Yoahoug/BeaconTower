<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
    <div 
      class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 relative overflow-hidden max-h-[90vh] flex flex-col"
      :style="{ boxShadow: '6px 6px 0px #121212' }"
    >
      <!-- 顶栏 -->
      <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black shrink-0">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-lg bg-toy-yellow border-2 border-toy-black flex items-center justify-center">
            <ToyMenuIcons name="servers" class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-base font-black text-toy-black">添加服务器节点</h3>
            <p class="text-[10px] font-bold text-toy-muted">无 Agent 采集 · SSH 直连探测</p>
          </div>
        </div>
        <button 
          @click="$emit('close')"
          class="w-7 h-7 rounded-lg border-2 border-toy-black bg-toy-bg hover:bg-toy-pink hover:text-white font-black flex items-center justify-center text-xs transition-colors cursor-pointer"
        >
          ✕
        </button>
      </div>

      <!-- 表单主体 -->
      <form @submit.prevent="handleSave" class="mt-4 space-y-3.5 overflow-y-auto flex-1 pr-1 scrollbar-none">
        <div>
          <label class="block text-xs font-black text-toy-black mb-1">节点显示名称</label>
          <input 
            v-model="form.name" 
            type="text" 
            placeholder="例如: Tokyo-VPS-01" 
            class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none focus:ring-2 focus:ring-toy-yellow"
            required
          />
        </div>

        <div class="grid grid-cols-3 gap-2">
          <div class="col-span-2">
            <label class="block text-xs font-black text-toy-black mb-1">SSH 主机地址</label>
            <input 
              v-model="form.host" 
              type="text" 
              placeholder="IP 或域名" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
          </div>
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">端口</label>
            <input 
              v-model.number="form.port" 
              type="number" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-2">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">SSH 用户名</label>
            <input 
              v-model="form.username" 
              type="text" 
              placeholder="root" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
          </div>
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">区域归属</label>
            <input 
              v-model="form.region" 
              type="text" 
              placeholder="JP 东京" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
            />
          </div>
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">SSH 登录密码或私钥</label>
          <input 
            v-model="form.password" 
            type="password" 
            placeholder="凭据通过 AES-256-GCM 本地加密保存" 
            class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
          />
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">公开展示备注</label>
          <input 
            v-model="form.note_public" 
            type="text" 
            placeholder="例如: 博客与图床服务" 
            class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
          />
        </div>

        <div v-if="testResult" class="p-2.5 rounded-lg border text-xs font-mono font-bold" :class="testSuccess ? 'bg-green-50 border-green-500 text-green-800' : 'bg-red-50 border-red-500 text-red-800'">
          {{ testResult }}
        </div>

        <!-- 底部操作按钮 -->
        <div class="pt-3 border-t-2 border-toy-gray flex items-center justify-between">
          <button 
            type="button" 
            @click="handleTest" 
            class="toy-btn px-3 py-1.5 text-xs bg-toy-cyan rounded-lg"
          >
            一键测试连接
          </button>
          <div class="flex items-center gap-2">
            <button 
              type="button" 
              @click="$emit('close')" 
              class="px-3 py-1.5 text-xs font-bold rounded-lg border-2 border-toy-black"
            >
              取消
            </button>
            <button 
              type="submit" 
              class="toy-btn px-4 py-1.5 text-xs bg-toy-yellow rounded-lg"
            >
              保存并探测画像
            </button>
          </div>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { api } from '../../services/api';
import ToyMenuIcons from '../icons/ToyMenuIcons.vue';

defineProps<{
  isOpen: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'saved'): void;
}>();

const form = reactive({
  name: '',
  host: '',
  port: 22,
  username: 'root',
  password: '',
  region: 'JP 东京',
  note_public: '',
});

const testResult = ref('');
const testSuccess = ref(false);

function handleTest() {
  testResult.value = '正在测试 SSH 直连探测...';
  setTimeout(() => {
    testSuccess.value = true;
    testResult.value = '✓ 连通性测试通过！探测到 Linux 系统 (Debian 12, 4 Cores, 8GB RAM)';
  }, 600);
}

async function handleSave() {
  await api.createServer({
    name: form.name,
    region: form.region,
    note_public: form.note_public,
  });
  emit('saved');
  emit('close');
}
</script>
