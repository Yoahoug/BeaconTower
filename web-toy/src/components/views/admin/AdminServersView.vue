<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-3 mb-2">
      <div class="space-y-1.5">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h2 class="text-xl font-black text-toy-black">节点管理</h2>
          <span class="toy-badge bg-toy-yellow text-[11px] px-2.5 py-0.5">管理员专区</span>
        </div>
        <p class="text-xs font-bold text-zinc-500">SSH 凭据 · 试连回读画像 · 排序/隐藏/暂停 · 重新定位 · 密钥自愈</p>
      </div>
      <button
        @click="openCreate"
        class="toy-btn px-4 py-1.5 bg-toy-yellow text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
      >
        <span>➕</span><span>添加节点</span>
      </button>
    </div>

    <!-- 加载 / 错误 / 空态 -->
    <div v-if="loading" class="toy-card p-5 bg-white space-y-3">
      <div v-for="i in 3" :key="`sk-${i}`" class="h-10 bg-toy-gray/40 rounded-lg animate-pulse"></div>
    </div>
    <div v-else-if="error" class="toy-card p-6 bg-red-50 border-2.5 border-red-400 text-center space-y-2">
      <p class="text-xs font-black text-red-700">⚠ {{ error }}</p>
      <button class="toy-btn px-3 py-1.5 bg-white text-xs rounded-lg cursor-pointer" @click="load">重试</button>
    </div>
    <div v-else-if="!rows.length" class="toy-card p-10 bg-white text-center space-y-2">
      <span class="text-3xl">🗄️</span>
      <h4 class="text-sm font-black text-toy-black">还没有纳管节点</h4>
      <p class="text-xs font-bold text-toy-muted">点右上「添加节点」，填 SSH 地址与账号即可（试连即装钥）。</p>
    </div>

    <!-- 服务器表格管理卡片 -->
    <div v-else class="toy-card p-5 bg-white space-y-4">
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b-2.5 border-toy-black text-zinc-600 font-black text-xs uppercase tracking-wider">
              <th class="pb-3">节点名称</th>
              <th class="pb-3">区域与标签</th>
              <th class="pb-3">公网 IP / 归属</th>
              <th class="pb-3">系统画像</th>
              <th class="pb-3">功耗计量</th>
              <th class="pb-3">SSH 凭据</th>
              <th class="pb-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y-2 divide-toy-black/10">
            <template v-for="s in rows" :key="s.id">
              <tr class="hover:bg-toy-bg/60 transition-colors" :class="!s.enabled ? 'opacity-60' : ''">
                <!-- 1. 节点名称 -->
                <td class="py-3.5 font-bold font-sans text-toy-black">
                  <div class="flex items-center gap-2 flex-wrap">
                    <span v-if="s.is_self" class="px-2 py-0.5 bg-toy-yellow text-[10px] font-black border border-toy-black rounded">本机</span>
                    <span class="text-sm font-black text-toy-black">{{ s.name }}</span>
                    <span v-if="s.hidden" class="px-2 py-0.5 bg-zinc-200 text-zinc-700 text-[10px] font-bold border border-toy-black rounded" title="游客页不展示">隐藏</span>
                    <span v-if="!s.enabled" class="px-2 py-0.5 bg-rose-500 text-white text-[10px] font-bold border border-toy-black rounded" title="已暂停采集">暂停</span>
                  </div>
                </td>

                <!-- 2. 区域与标签 -->
                <td class="py-3.5 font-sans">
                  <div class="flex items-center gap-1.5 flex-wrap">
                    <span class="toy-badge bg-toy-yellow/50 text-[11px] font-bold px-2 py-0.5" :title="s.region_source === 'manual' ? '手动指定' : '自动定位'">
                      {{ s.region || '未定位' }}
                    </span>
                    <span v-for="t in (s.tags || []).slice(0, 2)" :key="t" class="px-2 py-0.5 rounded bg-toy-bg border border-toy-black/30 text-[11px] font-bold text-zinc-700">
                      {{ t }}
                    </span>
                    <span v-if="(s.tags || []).length > 2" class="text-[11px] font-bold text-zinc-500">
                      +{{ s.tags.length - 2 }}
                    </span>
                  </div>
                </td>

                <!-- 3. 公网 IP / 归属 -->
                <td class="py-3.5">
                  <div class="font-mono font-extrabold text-xs text-toy-black">
                    {{ s.profile?.public_ip || '—' }}
                  </div>
                  <div class="text-[11px] font-bold text-zinc-600 mt-1 flex items-center gap-1.5">
                    <span class="w-1.5 h-1.5 rounded-full bg-zinc-400"></span>
                    <span>{{ [s.profile?.geo_country, s.profile?.geo_city].filter(Boolean).join(' · ') || '未定位' }}</span>
                  </div>
                </td>

                <!-- 4. 系统画像 -->
                <td class="py-3.5">
                  <div class="font-black text-xs text-toy-black flex items-center gap-1.5">
                    <span>{{ s.profile?.os_name || '—' }}</span>
                    <span class="font-mono text-[11px] text-zinc-600 font-bold">{{ s.profile?.os_version || '' }}</span>
                  </div>
                  <div class="text-[11px] font-mono font-bold text-zinc-700 mt-1">
                    {{ s.profile?.cpu_cores || '?' }}C / {{ formatBytes(s.profile?.mem_total || 0) }} / {{ formatBytes(s.profile?.disk_total || 0) }} · <span class="uppercase font-sans font-bold">{{ s.profile?.virt || '—' }}</span>
                  </div>
                </td>

                <!-- 5. 功耗计量 -->
                <td class="py-3.5 font-sans">
                  <span v-if="s.power?.rapl" class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-amber-100 border border-amber-800 text-amber-900 font-black text-[11px]">
                    ⚡ RAPL 可用
                  </span>
                  <span v-else class="text-zinc-400 font-bold text-xs" :title="s.power?.last_error || '未暴露 RAPL 功率计接口'">--</span>
                  <div v-if="s.power?.battery" class="text-[11px] font-bold text-zinc-600 mt-1">
                    电池机型 · 基础 {{ s.power.base_load_w || 0 }}W
                  </div>
                </td>

                <!-- 6. SSH 凭据 -->
                <td class="py-3.5">
                  <span v-if="s.is_self" class="inline-block px-2.5 py-0.5 bg-toy-bg border border-toy-black/30 rounded text-[11px] font-bold text-zinc-600">本地采集 (免 SSH)</span>
                  <template v-else>
                    <div class="font-mono font-extrabold text-xs text-toy-black">
                      {{ s.ssh?.username }}@{{ s.ssh?.host }}:{{ s.ssh?.port }}
                    </div>
                    <div class="text-[11px] font-bold mt-1 flex items-center gap-1" :class="s.ssh?.ssh_ready ? 'text-emerald-700' : 'text-rose-600'">
                      <span>{{ s.ssh?.ssh_ready ? '✓' : '⚠' }}</span>
                      <span>{{ s.ssh?.ssh_ready ? `${s.ssh?.auth_type === 'password' ? '密码' : '密钥'}就绪` : '凭据失联' }}</span>
                    </div>
                  </template>
                </td>

                <!-- 7. 操作按钮 -->
                <td class="py-3.5 text-right space-x-1.5 font-sans whitespace-nowrap">
                  <button class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer shadow-toy-xs" @click="expandedId = expandedId === s.id ? 0 : s.id">
                    {{ expandedId === s.id ? '收起' : '详情' }}
                  </button>
                  <button class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer shadow-toy-xs" @click="openEdit(s)">编辑</button>
                  <button
                    v-if="!s.is_self"
                    class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-toy-bg cursor-pointer disabled:opacity-50 shadow-toy-xs"
                    :disabled="relocatingId === s.id"
                    title="公网 IP 归属多源兜底重探，修「未定位」"
                    @click="relocate(s)"
                  >{{ relocatingId === s.id ? '定位中…' : '重新定位' }}</button>
                  <button class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-toy-cyan cursor-pointer shadow-toy-xs" @click="toggleEnabled(s)">
                    {{ s.enabled ? '暂停' : '启用' }}
                  </button>
                  <button
                    class="px-2.5 py-1 rounded-lg text-xs font-black border-2 border-toy-black bg-white hover:bg-red-50 text-red-600 cursor-pointer shadow-toy-xs"
                    :disabled="deletingId === s.id"
                    @click="handleDelete(s)"
                  >{{ deletingId === s.id ? '删除中…' : '删除' }}</button>
                </td>
              </tr>
              <tr v-if="expandedId === s.id" class="bg-toy-bg/60">
                <td colspan="7" class="p-3">
                  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 text-[11px] font-mono">
                    <div class="p-2.5 bg-white rounded-lg border-2 border-toy-black space-y-0.5">
                      <div class="font-black text-toy-black">公开备注</div>
                      <div class="text-toy-muted">{{ s.note_public || '—' }}</div>
                      <div class="font-black text-toy-black mt-1">私有备注（仅管理端）</div>
                      <div class="text-toy-muted">{{ s.note_private || '—' }}</div>
                    </div>
                    <div class="p-2.5 bg-white rounded-lg border-2 border-toy-black space-y-0.5">
                      <div class="font-black text-toy-black">管理信息</div>
                      <div class="text-toy-muted">排序权重: {{ s.sort_order }} · 游客{{ s.hidden ? '隐藏' : '可见' }} · 采集{{ s.enabled ? '启用' : '暂停' }}</div>
                      <div class="text-toy-muted">最近采集成功: {{ s.last_success_at ? formatAgo(Math.floor(Date.now() / 1000) - s.last_success_at) : '从未' }}</div>
                      <div class="text-toy-muted">录入时间: {{ s.created_at ? formatAgo(Math.floor(Date.now() / 1000) - s.created_at) : '—' }}</div>
                    </div>
                    <div class="p-2.5 bg-white rounded-lg border-2 border-toy-black space-y-0.5">
                      <div class="font-black text-toy-black">SSH 与主机指纹</div>
                      <div class="text-toy-muted truncate" :title="s.ssh?.host_key_fp">指纹: {{ s.ssh?.host_key_fp || '未记录（首次连接 TOFU）' }}</div>
                      <div class="text-toy-muted">私钥: {{ s.ssh?.has_key ? '面板已托管' : '无' }} · 密码: {{ s.ssh?.has_password ? '已存' : '未存' }}</div>
                      <div class="text-toy-muted">{{ s.profile?.hostname || '—' }} · {{ s.profile?.kernel || '—' }} · {{ s.profile?.arch || '—' }}</div>
                      <div class="text-toy-muted">CPU: {{ s.profile?.cpu_model || '—' }}</div>
                    </div>
                  </div>
                  <div class="mt-2 flex items-center gap-2 flex-wrap">
                    <button
                      v-if="!s.is_self"
                      class="toy-btn px-3 py-1 text-[10px] font-black bg-toy-cyan rounded-lg cursor-pointer"
                      @click="openRepair(s)"
                    >🔑 密钥失联？重输密码重建</button>
                    <button
                      v-if="s.power?.rapl || s.power?.battery"
                      class="toy-btn px-3 py-1 text-[10px] font-black bg-white border-2 border-toy-black rounded-lg cursor-pointer"
                      @click="openCalibration(s)"
                    >⚡ 功耗校准（基础功耗）</button>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 风格内消息条（替代原生 alert） -->
    <div v-if="notice" class="toy-card p-3 flex items-start justify-between gap-3" :class="noticeOk ? 'bg-toy-mint/40 border-2.5 border-toy-black' : 'bg-red-50 border-2.5 border-red-400'">
      <span class="text-xs font-bold" :class="noticeOk ? 'text-toy-black' : 'text-red-700'">{{ notice }}</span>
      <button @click="notice = ''" class="text-xs font-black cursor-pointer">✕</button>
    </div>

    <!-- 修复密钥弹窗 -->
    <div v-if="repairTarget" class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-5" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <h3 class="text-base font-black text-toy-black">修复密钥 · {{ repairTarget.name }}</h3>
        <p class="mt-1 text-[11px] font-bold text-toy-muted">
          重装系统/换机后面板密钥失联：重输一次 SSH 密码，面板重建专属 Ed25519 密钥并装机，随后仍走密钥登录（面板不存储密码）。
        </p>
        <input
          v-model="repairPassword"
          type="password"
          placeholder="SSH 密码（一次性）"
          autocomplete="off"
          class="mt-3 w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none font-mono"
        />
        <div class="mt-4 flex items-center justify-end gap-2">
          <button @click="repairTarget = null" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button
            :disabled="repairing || !repairPassword"
            class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-50"
            @click="doRepair"
          >{{ repairing ? '重建中…' : '重建密钥' }}</button>
        </div>
      </div>
    </div>

    <!-- 功耗校准弹窗 -->
    <div v-if="calibTarget" class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm">
      <div class="bg-white border-3 border-toy-black rounded-2xl max-w-sm w-full p-5" :style="{ boxShadow: '6px 6px 0px #121212' }">
        <h3 class="text-base font-black text-toy-black">功耗校准 · {{ calibTarget.name }}</h3>
        <p class="mt-1 text-[11px] font-bold text-toy-muted">
          基础功耗（磁盘/网卡/风扇等非 RAPL 覆盖部分）。电池机型可按放电实测校准；插电机型留 0 表示仅统计 RAPL 覆盖域。
        </p>
        <input
          v-model.number="calibWatts"
          type="number"
          min="0"
          step="0.1"
          class="mt-3 w-full px-3 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-xl focus:outline-none font-mono"
        />
        <div class="mt-4 flex items-center justify-end gap-2">
          <button @click="calibTarget = null" class="px-3 py-1.5 rounded-xl border-2 border-toy-black bg-white text-xs font-black hover:bg-toy-bg cursor-pointer">取消</button>
          <button
            :disabled="calibrating"
            class="toy-btn px-4 py-1.5 rounded-xl bg-toy-yellow text-xs font-black cursor-pointer disabled:opacity-50"
            @click="doCalibrate"
          >{{ calibrating ? '保存中…' : '保存校准' }}</button>
        </div>
      </div>
    </div>

    <!-- 添加 / 编辑节点弹窗 -->
    <ServerEditorModal
      :is-open="editorOpen"
      :server="editing"
      @close="editorOpen = false"
      @saved="handleSaved"
    />

    <!-- 风格内确认弹窗（替代原生 confirm） -->
    <div v-if="confirmText" class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-toy-black/50 backdrop-blur-sm">
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
import { ref, onMounted } from 'vue';
import type { AdminServerItem } from '../../../types/backend';
import { formatBytes, formatAgo } from '../../../services/format';
import { api } from '../../../services/api';
import ServerEditorModal from '../../modals/ServerEditorModal.vue';

const emit = defineEmits<{
  (e: 'refresh'): void;
}>();

const rows = ref<AdminServerItem[]>([]);
const loading = ref(true);
const error = ref('');
const expandedId = ref(0);
const relocatingId = ref<number | null>(null);
const deletingId = ref<number | null>(null);
const notice = ref('');
const noticeOk = ref(false);

// 编辑 / 新建
const editorOpen = ref(false);
const editing = ref<AdminServerItem | null>(null);

// 修复密钥
const repairTarget = ref<AdminServerItem | null>(null);
const repairPassword = ref('');
const repairing = ref(false);

// 功耗校准
const calibTarget = ref<AdminServerItem | null>(null);
const calibWatts = ref(0);
const calibrating = ref(false);

// 确认弹窗
const confirmText = ref('');
let confirmAction: (() => void | Promise<void>) | null = null;

async function load() {
  loading.value = true;
  error.value = '';
  try {
    rows.value = await api.getAdminServers();
  } catch (e) {
    error.value = e instanceof Error ? e.message : '节点清单加载失败';
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  editorOpen.value = true;
}

function openEdit(s: AdminServerItem) {
  editing.value = s;
  editorOpen.value = true;
}

async function handleSaved() {
  noticeOk.value = true;
  notice.value = '节点信息已保存，采集与公开页数据将按下一轮周期刷新。';
  await load();
  emit('refresh');
}

async function relocate(s: AdminServerItem) {
  relocatingId.value = s.id;
  notice.value = '';
  try {
    const r = await api.relocateServer(s.id);
    noticeOk.value = true;
    notice.value = `「${s.name}」重新定位完成${r.region ? `：${r.region}` : ''}${r.detail ? ` · ${r.detail}` : ''}`;
    await load();
    emit('refresh');
  } catch (e) {
    noticeOk.value = false;
    notice.value = e instanceof Error ? e.message : '重新定位失败';
  } finally {
    relocatingId.value = null;
  }
}

function toggleEnabled(s: AdminServerItem) {
  const next = !s.enabled;
  confirmText.value = next
    ? `确认恢复「${s.name}」的采集？恢复后指标与会话将重新上报。`
    : `确认暂停「${s.name}」的采集？暂停后不再连接该节点（数据保留，公开页显示为离线）。`;
  confirmAction = async () => {
    try {
      await api.updateServer(s.id, { enabled: next });
      noticeOk.value = true;
      notice.value = `「${s.name}」已${next ? '启用' : '暂停'}采集`;
      await load();
      emit('refresh');
    } catch (e) {
      noticeOk.value = false;
      notice.value = e instanceof Error ? e.message : '操作失败';
    }
  };
}

function handleDelete(s: AdminServerItem) {
  confirmText.value = `确认删除节点「${s.name}」及其所有历史指标数据？此操作不可撤销。`;
  confirmAction = async () => {
    deletingId.value = s.id;
    try {
      await api.deleteServer(s.id);
      noticeOk.value = true;
      notice.value = `节点「${s.name}」已删除`;
      await load();
      emit('refresh');
    } catch (e) {
      noticeOk.value = false;
      notice.value = e instanceof Error ? e.message : '删除失败';
    } finally {
      deletingId.value = null;
    }
  };
}

function openRepair(s: AdminServerItem) {
  repairTarget.value = s;
  repairPassword.value = '';
}

async function doRepair() {
  const t = repairTarget.value;
  if (!t || !repairPassword.value) return;
  repairing.value = true;
  try {
    const r = await api.rotateServerKey(t.id, repairPassword.value);
    repairTarget.value = null;
    noticeOk.value = true;
    notice.value = `「${t.name}」密钥已重建，后续仍走密钥登录${r.detail ? ` · ${r.detail}` : ''}`;
    await load();
  } catch (e) {
    noticeOk.value = false;
    notice.value = e instanceof Error ? e.message : '密钥重建失败';
  } finally {
    repairing.value = false;
  }
}

function openCalibration(s: AdminServerItem) {
  calibTarget.value = s;
  calibWatts.value = s.power?.base_load_w || 0;
}

async function doCalibrate() {
  const t = calibTarget.value;
  if (!t) return;
  calibrating.value = true;
  try {
    await api.recalibratePower(t.id, Number(calibWatts.value) || 0);
    calibTarget.value = null;
    noticeOk.value = true;
    notice.value = `「${t.name}」基础功耗已校准为 ${calibWatts.value} W`;
    await load();
  } catch (e) {
    noticeOk.value = false;
    notice.value = e instanceof Error ? e.message : '功耗校准失败';
  } finally {
    calibrating.value = false;
  }
}

function confirmYes() {
  confirmText.value = '';
  const a = confirmAction;
  confirmAction = null;
  if (a) void a();
}

onMounted(load);
</script>
