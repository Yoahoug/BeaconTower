<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm select-none">
    <div
      class="bg-white border-3 border-toy-black rounded-2xl max-w-lg w-full p-6 relative overflow-hidden max-h-[90vh] flex flex-col"
      :style="{ boxShadow: '6px 6px 0px #121212' }"
      role="dialog"
      aria-modal="true"
      :aria-label="isEdit ? '编辑服务器节点' : '添加服务器节点'"
    >
      <!-- 顶栏 -->
      <div class="flex items-center justify-between pb-3 border-b-2.5 border-toy-black shrink-0">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-lg bg-toy-yellow border-2 border-toy-black flex items-center justify-center">
            <ToyMenuIcons name="servers" class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-base font-black text-toy-black">{{ isEdit ? '编辑服务器节点' : '添加服务器节点' }}</h3>
            <p class="text-[10px] font-bold text-toy-muted">
              {{ isEdit ? '改元数据与 SSH 参数不影响已采集的历史数据' : '无 Agent 采集 · SSH 直连探测（试连即装钥）' }}
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
            <label class="block text-xs font-black text-toy-black mb-1">
              SSH 主机地址<span v-if="isSelf" class="text-toy-muted font-bold">（宿主地址，容器可达）</span>
            </label>
            <input
              v-model="form.host"
              type="text"
              placeholder="IP 或域名"
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
            <p v-if="sshSuggest" class="mt-1 text-[10px] font-bold text-toy-muted">
              建议宿主地址：<button type="button" class="underline cursor-pointer" @click="form.host = sshSuggest">{{ sshSuggest }}</button>
              （容器里 127.0.0.1 指向面板自身，填它连不上宿主）
            </p>
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
            <label class="block text-xs font-black text-toy-black mb-1">SSH 认证方式</label>
            <select
              v-model="form.auth_type"
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono cursor-pointer"
            >
              <option value="key">面板代管密钥（推荐）</option>
              <option value="password">密码</option>
            </select>
          </div>
        </div>

        <!-- 新建：密码一次性引导装钥；编辑：留空即不改动凭据 -->
        <div>
          <label class="block text-xs font-black text-toy-black mb-1">
            SSH 登录密码{{ isEdit ? '（留空不改；填写后试连即重建密钥）' : '（一次性引导）' }}
          </label>
          <input
            v-model="form.password"
            type="password"
            :placeholder="isEdit ? '仅在需要重建密钥时填写' : '试连成功后面板自动装钥，密码不落库'"
            autocomplete="off"
            :required="!isEdit && form.auth_type === 'password'"
            class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
          />
        </div>

        <div class="grid grid-cols-2 gap-2">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">区域归属</label>
            <input
              v-model="form.region"
              type="text"
              placeholder="留空按出口 IP 自动定位"
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
            />
          </div>
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">标签（逗号分隔）</label>
            <input
              v-model="form.tagsText"
              type="text"
              placeholder="核心, WG-Hub"
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
            />
          </div>
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">公开展示备注（游客可见）</label>
          <input
            v-model="form.note_public"
            type="text"
            placeholder="例如: 博客与图床服务"
            class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
          />
        </div>

        <div>
          <label class="block text-xs font-black text-toy-black mb-1">私有备注（仅管理端）</label>
          <input
            v-model="form.note_private"
            type="text"
            placeholder="例如: 到期日 / 采购渠道 / 续费提醒"
            class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
          />
        </div>

        <div class="grid grid-cols-3 gap-2 items-end">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">排序权重</label>
            <input
              v-model.number="form.sort_order"
              type="number"
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
            />
          </div>
          <label class="flex items-center gap-2 px-3 py-2 bg-toy-bg border-2 border-toy-black rounded-lg text-xs font-black cursor-pointer">
            <input v-model="form.hidden" type="checkbox" class="accent-black" />
            游客隐藏
          </label>
          <label class="flex items-center gap-2 px-3 py-2 bg-toy-bg border-2 border-toy-black rounded-lg text-xs font-black cursor-pointer">
            <input v-model="form.enabled" type="checkbox" class="accent-black" />
            启用采集
          </label>
        </div>

        <div v-if="testResult" class="p-2.5 rounded-lg border text-xs font-mono font-bold" :class="testSuccess ? 'bg-green-50 border-green-500 text-green-800' : 'bg-red-50 border-red-500 text-red-800'" role="status">
          {{ testResult }}
        </div>

        <!-- 画像预览（试连成功后回读） -->
        <div v-if="probePreview" class="p-2.5 rounded-lg bg-toy-bg border-2 border-toy-black text-[10px] font-mono font-bold text-toy-black space-y-0.5">
          <div>🖥 {{ probePreview.hostname || '未知主机' }} · {{ probePreview.os_name }} {{ probePreview.os_version }} · {{ probePreview.kernel }}</div>
          <div>⚙ {{ probePreview.cpu_model }} · {{ probePreview.cpu_cores }}C / {{ formatBytes(probePreview.mem_total || 0) }} / {{ formatBytes(probePreview.disk_total || 0) }} · {{ probePreview.virt || '—' }}</div>
          <div>⚡ RAPL {{ probePreview.power?.rapl ? '可用' : '不可用' }}<template v-if="probePreview.power?.battery"> · 电池机型</template></div>
        </div>

        <!-- 底部操作按钮 -->
        <div class="pt-3 border-t-2 border-toy-gray flex items-center justify-between flex-wrap gap-2">
          <button
            type="button"
            @click="handleTest"
            :disabled="busy"
            class="toy-btn px-3 py-1.5 text-xs bg-toy-cyan rounded-lg disabled:opacity-60 cursor-pointer"
          >
            {{ busy && testing ? '探测中…' : (isEdit ? '重输密码并重建密钥' : '一键测试连接（装钥）') }}
          </button>
          <div class="flex items-center gap-2">
            <button
              type="button"
              @click="$emit('close')"
              class="px-3 py-1.5 text-xs font-bold rounded-lg border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer"
            >
              取消
            </button>
            <button
              type="submit"
              :disabled="busy"
              class="toy-btn px-4 py-1.5 text-xs bg-toy-yellow rounded-lg disabled:opacity-60 cursor-pointer"
            >
              {{ busy && !testing ? '保存中…' : (isEdit ? '保存修改' : '保存并探测画像') }}
            </button>
          </div>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, watch } from 'vue';
import type { AdminServerItem } from '../../types/backend';
import { formatBytes } from '../../services/format';
import { api } from '../../services/api';
import ToyMenuIcons from '../icons/ToyMenuIcons.vue';

const props = defineProps<{
  isOpen: boolean;
  // 传入即为编辑模式；null/undefined = 新建
  server?: AdminServerItem | null;
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
  auth_type: 'key' as 'key' | 'password',
  password: '',
  region: '',
  tagsText: '',
  note_public: '',
  note_private: '',
  sort_order: 0,
  hidden: false,
  enabled: true,
});

const testResult = ref('');
const testSuccess = ref(false);
const busy = ref(false);
const testing = ref(false);
const panelPrivateKey = ref('');
const probePayload = ref<Record<string, unknown> | null>(null);

const isEdit = computed(() => !!props.server);
const isSelf = computed(() => !!props.server?.is_self);
const sshSuggest = computed(() => props.server?.ssh_suggest_host || '');

const probePreview = computed(() => {
  const p = probePayload.value as { profile?: Record<string, unknown> } | null;
  return (p?.profile || null) as null | {
    hostname?: string; os_name?: string; os_version?: string; kernel?: string;
    cpu_model?: string; cpu_cores?: number; mem_total?: number; disk_total?: number; virt?: string;
    power?: { rapl?: boolean; battery?: boolean };
  };
});

// 打开时按模式回填
watch(() => [props.isOpen, props.server] as const, () => {
  if (!props.isOpen) return;
  testResult.value = '';
  testSuccess.value = false;
  panelPrivateKey.value = '';
  probePayload.value = null;
  const s = props.server;
  form.name = s?.name || '';
  form.host = s?.ssh?.host || '';
  form.port = s?.ssh?.port || 22;
  form.username = s?.ssh?.username || 'root';
  form.auth_type = (s?.ssh?.auth_type === 'password' ? 'password' : 'key');
  form.password = '';
  form.region = s?.region || '';
  form.tagsText = (s?.tags || []).join(', ');
  form.note_public = s?.note_public || '';
  form.note_private = s?.note_private || '';
  form.sort_order = s?.sort_order || 0;
  form.hidden = !!s?.hidden;
  form.enabled = s?.enabled !== false;
}, { immediate: true });

function sshPayload(): Record<string, unknown> {
  const base: Record<string, unknown> = {
    host: form.host.trim(),
    port: Number(form.port) || 22,
    username: form.username.trim() || 'root',
    auth_type: form.password ? 'password' : form.auth_type,
  };
  if (form.password) base.password = form.password;
  return base;
}

function metaPayload(): Record<string, unknown> {
  const tags = form.tagsText.split(/[,，]/).map((t) => t.trim()).filter(Boolean);
  return {
    name: form.name.trim(),
    region: form.region.trim(),
    region_source: form.region.trim() ? 'manual' : 'auto',
    tags,
    note_public: form.note_public,
    note_private: form.note_private,
    sort_order: Number(form.sort_order) || 0,
    hidden: form.hidden,
    enabled: form.enabled,
  };
}

// 试连即装钥：新建时用 bootstrap 引导；编辑时走 rotate-key 重建
async function handleTest() {
  busy.value = true;
  testing.value = true;
  testResult.value = '正在建立 SSH 直连并回读系统画像…';
  testSuccess.value = false;
  try {
    if (isEdit.value && props.server) {
      if (!form.password) throw new Error('编辑模式下重建密钥需要填写一次 SSH 密码');
      const r = await api.rotateServerKey(props.server.id, form.password);
      testSuccess.value = true;
      testResult.value = `✓ 密钥已重建${r.detail ? `：${r.detail}` : ''}`;
    } else {
      const r = await api.testConnectionBootstrap({
        host: form.host.trim(),
        port: Number(form.port) || 22,
        username: form.username.trim() || 'root',
        auth_type: 'password',
        password: form.password,
      });
      testSuccess.value = true;
      panelPrivateKey.value = r.panelPrivateKey;
      probePayload.value = r.probe;
      testResult.value = `✓ ${r.msg}`;
    }
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
    if (isEdit.value && props.server) {
      const payload: Record<string, unknown> = { ...metaPayload() };
      // SSH 参数有改动时才下发；密码只用于 rotate-key，绝不随元数据落库
      if (form.host.trim() && form.host.trim() !== props.server.ssh?.host) {
        payload.ssh = {
          host: form.host.trim(),
          port: Number(form.port) || 22,
          username: form.username.trim() || 'root',
          auth_type: form.auth_type,
        };
      }
      await api.updateServer(props.server.id, payload);
    } else {
      await api.createServer({
        name: form.name.trim(),
        region: form.region.trim() || undefined,
        note_public: form.note_public,
        ssh: sshPayload(),
        panel_private_key: panelPrivateKey.value || undefined,
        probe: probePayload.value ?? undefined,
      });
    }
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
