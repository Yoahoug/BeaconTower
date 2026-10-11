<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>节点管理</span>
          <span class="toy-badge bg-toy-yellow text-[10px]">管理员专区</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">SSH 凭据录入（添加节点请在节点编辑弹窗完成）· 试连 · 删除</p>
      </div>
    </div>

    <!-- 服务器表格管理卡片 -->
    <div class="toy-card p-5 bg-white space-y-4">
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
              <th class="pb-2">节点名称</th>
              <th class="pb-2">区域与标签</th>
              <th class="pb-2">系统画像</th>
              <th class="pb-2">功耗计量</th>
              <th class="pb-2">SSH 凭据</th>
              <th class="pb-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-toy-gray font-mono">
            <tr v-for="s in rows" :key="s.id" class="hover:bg-toy-bg/50 transition-colors">
              <td class="py-3 font-bold font-sans text-toy-black flex items-center gap-2">
                <span v-if="s.is_self" class="px-1.5 py-0.2 bg-toy-yellow text-[9px] font-black border border-toy-black rounded">本机</span>
                <span>{{ s.name }}</span>
              </td>
              <td class="py-3">
                <span class="toy-badge bg-toy-yellow/40 text-[10px]">{{ s.region || '未定位' }}</span>
                <span v-for="t in (s.tags || []).slice(0, 2)" :key="t" class="ml-1 px-1.5 py-0.5 rounded bg-toy-bg border border-toy-gray text-[10px]">{{ t }}</span>
              </td>
              <td class="py-3 text-toy-muted">
                {{ s.profile?.os || '—' }} · {{ s.profile?.cpu_cores || '?' }}C / {{ formatBytes(s.profile?.mem_total || 0) }}
              </td>
              <td class="py-3">
                <span v-if="s.power" class="text-amber-800 font-bold">⚡ {{ s.power.total_w }} W</span>
                <span v-else class="text-toy-muted">--</span>
              </td>
              <td class="py-3">
                <span v-if="s.is_self" class="text-toy-muted">本地采集</span>
                <span v-else-if="s.ssh?.ssh_ready" class="text-toy-mint font-bold">✓ 就绪</span>
                <span v-else class="text-toy-muted">未录入 / 失联</span>
              </td>
              <td class="py-3 text-right space-x-1.5">
                <button
                  @click="handleTestConnect(s)"
                  class="px-2 py-1 rounded text-[10px] font-bold border border-toy-black bg-white hover:bg-toy-bg transition-colors cursor-pointer"
                  title="SSH 连通性测试"
                >
                  {{ testingId === s.id ? '试连中…' : '试连' }}
                </button>
                <button
                  @click="handleDelete(s)"
                  class="px-2 py-1 rounded text-[10px] font-bold border border-toy-black bg-white hover:bg-red-50 text-red-600 transition-colors cursor-pointer"
                >
                  删除
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 风格内消息条（替代原生 alert） -->
    <div v-if="notice" class="toy-card p-3 flex items-start justify-between gap-3" :class="noticeOk ? 'bg-toy-mint-light border-2.5 border-toy-black' : 'bg-red-50 border-2.5 border-red-400'">
      <span class="text-xs font-bold" :class="noticeOk ? 'text-toy-black' : 'text-red-700'">{{ notice }}</span>
      <button @click="notice = ''" class="text-xs font-black cursor-pointer">✕</button>
    </div>

    <!-- 风格内确认弹窗（替代原生 confirm） -->
    <div v-if="confirmText" class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-xs">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-5" :style="{ boxShadow: '6px 6px 0px #121212' }" role="alertdialog" aria-modal="true">
        <h3 class="text-base font-black text-toy-black flex items-center gap-2"><span>⚠️</span>请确认</h3>
        <p class="mt-2 text-xs font-bold text-toy-muted">{{ confirmText }}</p>
        <div class="mt-4 flex items-center justify-end gap-2">
          <button @click="confirmText = ''" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button @click="confirmYes" class="toy-btn px-4 py-1.5 rounded-xl bg-toy-pink text-white text-xs font-black cursor-pointer">确认</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import type { ServerItem } from '../../../types/backend';

// 管理行扩展：ssh 凭据就绪位来自 /admin/servers（公开 ServerItem 无此字段）
interface AdminRow extends Partial<ServerItem> {
  id: number;
  name: string;
  is_self?: boolean;
  ssh?: { ssh_ready?: boolean; host?: string; port?: number; username?: string };
  profile?: ServerItem['profile'];
  power?: ServerItem['power'];
  tags?: string[];
  region?: string;
}
import { formatBytes } from '../../../services/format';
import { api } from '../../../services/api';

const props = defineProps<{
  servers: ServerItem[];
}>();

const rows = computed<AdminRow[]>(() => props.servers as AdminRow[]);

const emit = defineEmits<{
  (e: 'refresh'): void;
}>();

const testingId = ref<number | null>(null);
const notice = ref('');
const noticeOk = ref(false);
const confirmText = ref('');
let confirmAction: (() => void | Promise<void>) | null = null;

async function handleTestConnect(s: AdminRow) {
  testingId.value = s.id;
  notice.value = '';
  try {
    const r = await api.testConnection(s.name);
    noticeOk.value = r.ok;
    notice.value = `${r.ok ? '✓' : '✕'} ${r.msg}`;
  } finally {
    testingId.value = null;
  }
}

function handleDelete(s: AdminRow) {
  confirmText.value = `确认删除节点「${s.name}」及其所有历史指标数据？此操作不可撤销。`;
  confirmAction = async () => {
    try {
      await api.deleteServer(s.id);
      noticeOk.value = true;
      notice.value = `节点「${s.name}」已删除`;
      emit('refresh');
    } catch (e) {
      noticeOk.value = false;
      notice.value = e instanceof Error ? e.message : '删除失败';
    }
  };
}

function confirmYes() {
  confirmText.value = '';
  const a = confirmAction;
  confirmAction = null;
  if (a) void a();
}

// 供父级校准：props 仅读
void props;
</script>
