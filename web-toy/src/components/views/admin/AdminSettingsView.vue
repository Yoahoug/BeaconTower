<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-3 mb-2">
      <div class="space-y-1.5">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h2 class="text-xl font-black text-toy-black">采集与展示</h2>
          <span class="toy-badge bg-toy-yellow text-[11px] px-2.5 py-0.5">系统参数</span>
        </div>
        <p class="text-xs font-bold text-zinc-500">步频周期 / 功耗展示 / 公开页白名单与 SSH 校验策略</p>
      </div>

      <button 
        @click="handleSave"
        class="toy-btn px-4 py-1.5 bg-toy-yellow text-xs font-black rounded-xl flex items-center gap-1.5 cursor-pointer"
        :disabled="isSaving"
      >
        <span>{{ isSaving ? '⏳' : isSaved ? '✔' : '💾' }}</span>
        <span>{{ isSaving ? '正在保存...' : isSaved ? '已保存！' : '保存系统设置' }}</span>
      </button>
    </div>

    <!-- 表单卡片堆叠 -->
    <div class="space-y-5 max-w-4xl">
      <!-- 1. 采集参数 -->
      <div class="toy-card p-6 bg-white space-y-4">
        <div class="flex items-center gap-2.5 border-b-2 border-toy-black/15 pb-3">
          <span class="text-lg">⏱️</span>
          <h3 class="text-sm font-black text-toy-black">采集步频与数据保留</h3>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-6 pt-1">
          <div>
            <label class="block text-xs font-black text-toy-black mb-2">采集间隔（秒，≥5）</label>
            <input 
              v-model.number="form.interval_s"
              type="number" 
              min="5" 
              max="300"
              class="w-full px-3.5 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
            <span class="text-[11px] text-zinc-500 font-bold mt-2 block leading-relaxed">
              默认 10 秒；间隔越小曲线越细腻，目标机开销略微增加。
            </span>
          </div>

          <div>
            <label class="block text-xs font-black text-toy-black mb-2">历史数据保留时长（天）</label>
            <input 
              v-model.number="form.retention_days"
              type="number" 
              min="1" 
              max="365"
              class="w-full px-3.5 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
            <span class="text-[11px] text-zinc-500 font-bold mt-2 block leading-relaxed">
              按天聚合存入 SQLite，超期样本由后台任务自动清理。
            </span>
          </div>
        </div>
      </div>

      <!-- 2. 功耗计量与电费估算 -->
      <div class="toy-card p-6 bg-white space-y-4">
        <div class="flex items-center gap-2.5 border-b-2 border-toy-black/15 pb-3">
          <span class="text-lg">⚡</span>
          <h3 class="text-sm font-black text-toy-black">功耗展示与电费单价</h3>
        </div>

        <div class="space-y-4 pt-1">
          <label class="flex items-start gap-3 cursor-pointer p-2.5 -mx-2 rounded-lg hover:bg-toy-bg/60 transition-colors">
            <input 
              v-model="form.show_power_public"
              type="checkbox" 
              class="w-4 h-4 mt-0.5 rounded border-2 border-toy-black text-toy-black focus:ring-0 cursor-pointer accent-toy-black shrink-0"
            />
            <div class="space-y-1">
              <span class="text-xs font-black text-toy-black block">公开页显示实时功耗数据</span>
              <span class="text-[11px] text-zinc-500 font-bold block leading-relaxed">
                展示实时整机功率 (W)、月度用电量 (kWh)；未暴露 RAPL 的机型自动隐藏。
              </span>
            </div>
          </label>

          <label class="flex items-start gap-3 cursor-pointer p-2.5 -mx-2 rounded-lg hover:bg-toy-bg/60 transition-colors">
            <input 
              v-model="form.show_cost_public"
              type="checkbox" 
              class="w-4 h-4 mt-0.5 rounded border-2 border-toy-black text-toy-black focus:ring-0 cursor-pointer accent-toy-black shrink-0"
            />
            <div class="space-y-1">
              <span class="text-xs font-black text-toy-black block">公开页显示电费估算</span>
              <span class="text-[11px] text-zinc-500 font-bold block leading-relaxed">
                根据月度用电量与下方设定的基准单价自动折算月度预估电费。
              </span>
            </div>
          </label>

          <div class="pt-3 max-w-xs border-t border-toy-black/10">
            <label class="block text-xs font-black text-toy-black mb-2">基准电价（元 / kWh）</label>
            <div class="relative">
              <input 
                v-model.number="form.electric_price"
                type="number" 
                step="0.01"
                min="0"
                max="99"
                class="w-full px-3.5 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              />
              <span class="absolute right-3 top-2 text-xs text-zinc-500 font-bold">¥/度</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 3. 公开页面与访问控制 -->
      <div class="toy-card p-6 bg-white space-y-4">
        <div class="flex items-center gap-2.5 border-b-2 border-toy-black/15 pb-3">
          <span class="text-lg">🌐</span>
          <h3 class="text-sm font-black text-toy-black">公开页面控制</h3>
        </div>

        <div class="space-y-4 pt-1">
          <div>
            <label class="block text-xs font-black text-toy-black mb-2">控制台站点标题</label>
            <input 
              v-model="form.site_title"
              type="text" 
              class="w-full px-3.5 py-2 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none"
            />
          </div>

          <label class="flex items-start gap-3 cursor-pointer p-2.5 -mx-2 rounded-lg hover:bg-toy-bg/60 transition-colors">
            <input 
              v-model="form.open_7d_history"
              type="checkbox" 
              class="w-4 h-4 mt-0.5 rounded border-2 border-toy-black text-toy-black focus:ring-0 cursor-pointer accent-toy-black shrink-0"
            />
            <div class="space-y-1">
              <span class="text-xs font-black text-toy-black block">对访客开放 7 天历史趋势曲线</span>
              <span class="text-[11px] text-zinc-500 font-bold block leading-relaxed">
                未勾选时访客仅可查看最近 1 小时与 6 小时走势。
              </span>
            </div>
          </label>

          <label class="flex items-start gap-3 cursor-pointer p-2.5 -mx-2 rounded-lg hover:bg-toy-bg/60 transition-colors">
            <input 
              v-model="form.private_mode"
              type="checkbox" 
              class="w-4 h-4 mt-0.5 rounded border-2 border-toy-black text-toy-black focus:ring-0 cursor-pointer accent-toy-black shrink-0"
            />
            <div class="space-y-1">
              <span class="text-xs font-black text-toy-black block">完全私有化模式（Private Mode）</span>
              <span class="text-[11px] text-rose-600 font-bold block leading-relaxed">
                开启后所有公开 API 与页面均强制要求管理员登录，未登录直接拦截跳转。
              </span>
            </div>
          </label>
        </div>
      </div>

      <!-- 4. SSH 安全策略 -->
      <div class="toy-card p-6 bg-white space-y-4">
        <div class="flex items-center gap-2.5 border-b-2 border-toy-black/15 pb-3">
          <span class="text-lg">🛡️</span>
          <h3 class="text-sm font-black text-toy-black">SSH 传输安全</h3>
        </div>

        <div class="pt-1">
          <label class="flex items-start gap-3 cursor-pointer p-2.5 -mx-2 rounded-lg hover:bg-toy-bg/60 transition-colors">
            <input 
              v-model="form.strict_host_key"
              type="checkbox" 
              class="w-4 h-4 mt-0.5 rounded border-2 border-toy-black text-toy-black focus:ring-0 cursor-pointer accent-toy-black shrink-0"
            />
            <div class="space-y-1">
              <span class="text-xs font-black text-toy-black block">Host Key 严格校验 (TOFU 机制)</span>
              <span class="text-[11px] text-zinc-500 font-bold block leading-relaxed">
                当远程服务器公钥指纹与初次连接记录不符时拒绝采集，防范中间人劫持与 DNS 欺骗。
              </span>
            </div>
          </label>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import type { SystemSettingsData } from '../../../types/backend';
import { api } from '../../../services/api';

const form = ref<SystemSettingsData>({
  site_title: '信标塔 · BeaconTower',
  interval_s: 10,
  retention_days: 30,
  open_7d_history: false,
  show_power_public: true,
  show_cost_public: true,
  electric_price: 0.65,
  strict_host_key: true,
  private_mode: false,
});

const isSaving = ref(false);
const isSaved = ref(false);
const errorMsg = ref('');

async function loadData() {
  try {
    form.value = await api.getSettings();
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '读取设置失败';
  }
}

async function handleSave() {
  if (form.value.interval_s < 5) {
    errorMsg.value = '采集间隔不能小于 5 秒！';
    return;
  }
  isSaving.value = true;
  errorMsg.value = '';
  try {
    await api.saveSettings(form.value);
    isSaved.value = true;
    setTimeout(() => {
      isSaved.value = false;
    }, 2500);
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '保存失败';
  } finally {
    isSaving.value = false;
  }
}

onMounted(() => {
  loadData();
});
</script>
