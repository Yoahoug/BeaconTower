<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>审计日志</span>
          <span class="toy-badge bg-toy-yellow text-[10px]">不可篡改</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">管理操作留痕 · 来源 IP 哈希脱敏 · 历史追踪</p>
      </div>

      <div class="flex items-center gap-2">
        <div class="relative min-w-[200px]">
          <input 
            v-model="searchQuery"
            @input="handleSearch"
            type="text" 
            placeholder="搜索操作 / 对象 / 操作者..." 
            class="w-full px-3 py-1.5 text-xs font-bold bg-white border-2 border-toy-black rounded-lg focus:outline-none focus:ring-2 focus:ring-toy-yellow shadow-toy-sm text-toy-black placeholder-toy-muted"
          />
          <span v-if="searchQuery" @click="clearSearch" class="absolute right-2 top-1.5 text-xs cursor-pointer text-toy-muted font-black">✕</span>
        </div>
      </div>
    </div>

    <!-- 审计日志表格 -->
    <div class="toy-card p-5 bg-white space-y-4">
      <div class="flex items-center justify-between text-xs font-mono">
        <span class="text-toy-muted font-bold">共 {{ total }} 条记录 · 来源 IP 仅以哈希存储展示（SHA-256）</span>
        <span class="text-toy-black font-extrabold">第 {{ page }} / {{ pages }} 页</span>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
              <th class="pb-2">发生时间</th>
              <th class="pb-2">操作动作</th>
              <th class="pb-2">目标对象</th>
              <th class="pb-2">操作明细内容</th>
              <th class="pb-2">操作者</th>
              <th class="pb-2 text-right">来源 IP</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-toy-gray font-mono">
            <tr v-for="log in logs" :key="log.id" class="hover:bg-toy-bg/50 transition-colors">
              <td class="py-3 text-toy-muted font-bold whitespace-nowrap">{{ formatTime(log.ts) }}</td>
              <td class="py-3">
                <span 
                  class="toy-badge text-[10px]"
                  :class="getActionBadgeClass(log.action)"
                >
                  {{ getActionLabel(log.action) }}
                </span>
              </td>
              <td class="py-3 font-bold text-toy-black whitespace-nowrap">{{ log.target }}</td>
              <td class="py-3 text-toy-black font-medium min-w-[200px]">{{ log.detail }}</td>
              <td class="py-3 font-bold text-toy-black whitespace-nowrap">{{ log.actor }}</td>
              <td class="py-3 text-right text-toy-muted">{{ log.source_ip_hash }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 分页控制器 -->
      <div v-if="pages > 1" class="flex items-center justify-between pt-2 border-t-2 border-toy-black">
        <button 
          @click="changePage(page - 1)"
          :disabled="page <= 1"
          class="toy-btn px-3 py-1 bg-white hover:bg-toy-yellow text-xs font-black rounded-lg disabled:opacity-40 disabled:hover:bg-white cursor-pointer"
        >
          ◀ 上一页
        </button>
        <span class="text-xs font-mono font-black text-toy-black">{{ page }} / {{ pages }}</span>
        <button 
          @click="changePage(page + 1)"
          :disabled="page >= pages"
          class="toy-btn px-3 py-1 bg-white hover:bg-toy-yellow text-xs font-black rounded-lg disabled:opacity-40 disabled:hover:bg-white cursor-pointer"
        >
          下一页 ▶
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { AuditLogItem } from '../../../types/backend';
import { api } from '../../../services/api';

const logs = ref<AuditLogItem[]>([]);
const total = ref(0);
const page = ref(1);
const pages = ref(1);
const searchQuery = ref('');

const ACTION_MAP: Record<string, { label: string; cls: string }> = {
  setup: { label: '初始化', cls: 'bg-toy-mint text-toy-black' },
  login: { label: '登录', cls: 'bg-toy-mint text-toy-black' },
  server_create: { label: '新增节点', cls: 'bg-toy-yellow text-toy-black' },
  server_update: { label: '更新节点', cls: 'bg-toy-bg text-toy-black' },
  server_delete: { label: '删除节点', cls: 'bg-red-100 text-red-700' },
  server_test: { label: 'SSH 探测', cls: 'bg-sky-100 text-sky-800' },
  power_calibrate: { label: '功耗校准', cls: 'bg-toy-yellow/60 text-toy-black' },
  settings: { label: '系统设置', cls: 'bg-purple-100 text-purple-800' },
  wg_patrol: { label: '星网巡检', cls: 'bg-toy-mint/60 text-toy-black' },
};

function getActionLabel(action: string) {
  return ACTION_MAP[action]?.label || action;
}

function getActionBadgeClass(action: string) {
  return ACTION_MAP[action]?.cls || 'bg-toy-bg text-toy-black';
}

function formatTime(ts: number) {
  const d = new Date(ts * 1000);
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  const h = String(d.getHours()).padStart(2, '0');
  const min = String(d.getMinutes()).padStart(2, '0');
  const s = String(d.getSeconds()).padStart(2, '0');
  return `${m}-${day} ${h}:${min}:${s}`;
}

async function loadData() {
  const res = await api.getAuditLogs(searchQuery.value, page.value, 10);
  logs.value = res.list;
  total.value = res.total;
  page.value = res.page;
  pages.value = res.pages;
}

function handleSearch() {
  page.value = 1;
  loadData();
}

function clearSearch() {
  searchQuery.value = '';
  page.value = 1;
  loadData();
}

function changePage(newPage: number) {
  if (newPage >= 1 && newPage <= pages.value) {
    page.value = newPage;
    loadData();
  }
}

onMounted(() => {
  loadData();
});
</script>
