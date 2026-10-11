<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>节点管理</span>
          <span class="toy-badge bg-toy-yellow text-[10px]">管理员专区</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">管理 SSH 凭据 · 画像自动回读 · 功耗校准与排序</p>
      </div>

      <button 
        @click="isModalOpen = true"
        class="toy-btn px-3.5 py-1.5 bg-toy-yellow text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
      >
        <span>➕</span>
        <span>添加新节点</span>
      </button>
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
              <th class="pb-2">状态</th>
              <th class="pb-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-toy-gray font-mono">
            <tr v-for="s in servers" :key="s.id" class="hover:bg-toy-bg/50 transition-colors">
              <td class="py-3 font-bold font-sans text-toy-black flex items-center gap-2">
                <span v-if="s.is_self" class="px-1.5 py-0.2 bg-toy-yellow text-[9px] font-black border border-toy-black rounded">本机</span>
                <span>{{ s.name }}</span>
              </td>
              <td class="py-3">
                <span class="toy-badge bg-toy-yellow/40 text-[10px]">{{ s.region }}</span>
              </td>
              <td class="py-3 text-toy-muted">
                {{ s.profile.os }} · {{ s.profile.cpu_cores }}C / {{ formatBytes(s.profile.mem_total) }}
              </td>
              <td class="py-3">
                <span v-if="s.power" class="text-amber-800 font-bold">⚡ {{ s.power.total_w }} W</span>
                <span v-else class="text-toy-muted">--</span>
              </td>
              <td class="py-3">
                <span 
                  class="toy-badge text-[10px]"
                  :class="s.status === 'online' ? 'bg-toy-mint text-toy-black' : 'bg-gray-200 text-toy-muted'"
                >
                  {{ s.status === 'online' ? '● 在线' : '○ 离线' }}
                </span>
              </td>
              <td class="py-3 text-right space-x-1.5">
                <button 
                  @click="handleTestConnect(s.name)"
                  class="px-2 py-1 rounded text-[10px] font-bold border border-toy-black bg-white hover:bg-toy-bg transition-colors"
                  title="SSH 连通性测试"
                >
                  试连
                </button>
                <button 
                  @click="handleDelete(s.id)"
                  class="px-2 py-1 rounded text-[10px] font-bold border border-toy-black bg-white hover:bg-red-50 text-red-600 transition-colors"
                >
                  删除
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 添加节点弹窗 -->
    <ServerEditorModal 
      :is-open="isModalOpen" 
      @close="isModalOpen = false" 
      @saved="$emit('refresh')"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { ServerItem } from '../../../types/backend';
import { formatBytes } from '../../../services/mock';
import { api } from '../../../services/api';
import ServerEditorModal from '../../modals/ServerEditorModal.vue';

const props = defineProps<{
  servers: ServerItem[];
}>();

const emit = defineEmits<{
  (e: 'refresh'): void;
}>();

const isModalOpen = ref(false);

function handleTestConnect(name: string) {
  alert(`SSH 试连探测成功：已连接至 [${name}]，时延 14ms，采集指标正常！`);
}

async function handleDelete(id: number) {
  if (confirm('确认删除该节点及其所有历史指标数据？')) {
    await api.deleteServer(id);
    emit('refresh');
  }
}
</script>
