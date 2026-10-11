<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs select-none">
    <div
      class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 relative overflow-hidden max-h-[90vh] flex flex-col"
      :style="{ boxShadow: '6px 6px 0px #121212' }"
      role="dialog"
      aria-modal="true"
      aria-label="添加服务器节点"
    >
      <!-- 顶栏 -->
      <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black shrink-0">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-lg bg-toy-yellow border-2 border-toy-black flex items-center justify-center">
            <ToyMenuIcons name="servers" class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-base font-black text-toy-black">添加服务器节点</h3>
            <p class="text-[10px] font-bold text-toy-muted">无 Agent 采集 · SSH 直连探测（试连即装钥）</p>
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
              min="1"
              max="65535"
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
            <label class="block text-xs font-black text-toy-black mb-1">区域归属（可留空自动定位）</label>
            <input
              v-model="form.region"
              type="text"
              placeholder="留空则按出口 IP 自动定位"
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
            />
          </div>
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">SSH 登录密码（一次性引导）</label>
          <input
            v-model="form.password"
            type="password"
            placeholder="试连成功后面板自动装钥，密码不落库"
            autocomplete="off"
            class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
            required
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

        <div v-if="testResult" class="p-2.5 rounded-lg border text-xs font-mono font-bold" :class="testSuccess ? 'bg-green-50 border-green-500 text-green-800' : 'bg-red-50 border-red-500 text-red-800'" role="status">
          {{ testResult }}
        </div>

        <!-- 底部操作按钮 -->
        <div class="pt-3 border-t-2 border-toy-gray flex items-center justify-between">
          <button
            type="button"
            @click="handleTest"
            :disabled="busy"
            class="toy-btn px-3 py-1.5 text-xs bg-toy-cyan rounded-lg disabled:opacity-60"
          >
            {{ busy && testing ? '探测中…' : '一键测试连接（装钥）' }}
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
              :disabled="busy"
              class="toy-btn px-4 py-1.5 text-xs bg-toy-yellow rounded-lg disabled:opacity-60"
            >
              {{ busy && !testing ? '保存中…' : '保存并探测画像' }}
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
  region: '',
  note_public: '',
});

const testResult = ref('');
const testSuccess = ref(false);
const busy = ref(false);
const testing = ref(false);
// 试连成功后面板私钥只存内存，随保存请求原样带回（后端不落库密码/私钥缓存）
const panelPrivateKey = ref('');
const probePayload = ref<Record<string, unknown> | null>(null);

function sshPayload() {
  return {
    host: form.host.trim(),
    port: Number(form.port) || 22,
    username: form.username.trim() || 'root',
    auth_type: 'password',
    password: form.password,
  };
}

async function handleTest() {
  testResult.value = '正在测试 SSH 直连探测（成功后自动安装面板密钥）…';
  testSuccess.value = false;
  busy.value = true;
  testing.value = true;
  try {
    const r = await api.testConnectionBootstrap(sshPayload());
    testSuccess.value = true;
    panelPrivateKey.value = r.panelPrivateKey;
    probePayload.value = r.probe;
    testResult.value = `✓ ${r.msg}`;
  } catch (e) {
    testSuccess.value = false;
    testResult.value = e instanceof Error ? e.message : '试连失败';
  } finally {
    busy.value = false;
    testing.value = false;
  }
}

async function handleSave() {
  busy.value = true;
  try {
    await api.createServer({
      name: form.name.trim(),
      region: form.region.trim() || undefined,
      note_public: form.note_public,
      ssh: sshPayload(),
      panel_private_key: panelPrivateKey.value || undefined,
      probe: probePayload.value ?? undefined,
    });
    emit('saved');
    emit('close');
  } catch (e) {
    testSuccess.value = false;
    testResult.value = e instanceof Error ? e.message : '保存失败';
  } finally {
    busy.value = false;
  }
}
</script>
