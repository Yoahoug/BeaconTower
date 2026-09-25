<script setup>
// 采集与展示设置：采集间隔、保留策略、公开字段开关、电价、host key 严格模式。
import { ref, onMounted } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { adminApi } from './store'

const settings = ref(null)
const saving = ref(false)
const saved = ref(false)

onMounted(async () => {
  settings.value = await adminApi.getSettings()
})

async function save() {
  saving.value = true
  try {
    await adminApi.saveSettings({ ...settings.value })
    saved.value = true
    setTimeout(() => (saved.value = false), 2000)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div v-if="settings" class="settings-admin">
    <div class="glass-card settings-card">
      <h4 class="sec-title">采集</h4>
      <div class="form-grid">
        <label class="field">
          <span class="field-label">采集间隔（秒，≥5）</span>
          <input v-model.number="settings.interval_s" type="number" min="5" max="300" />
        </label>
        <label class="field">
          <span class="field-label">历史数据保留（天）</span>
          <input v-model.number="settings.retention_days" type="number" min="1" max="365" />
        </label>
      </div>
      <p class="sec-hint">面板按此间隔 SSH 拉取一轮指标（含功耗采样），间隔越小曲线越细、目标机开销略增。</p>
    </div>

    <div class="glass-card settings-card">
      <h4 class="sec-title">功耗展示</h4>
      <div class="form-grid">
        <label class="check-field field-wide">
          <input v-model="settings.show_power_public" type="checkbox" />
          <span>公开页显示功耗数据（功率 / kWh / 走势；未暴露 RAPL 的机型自动显示"不可用"）</span>
        </label>
        <label class="check-field field-wide">
          <input v-model="settings.show_cost_public" type="checkbox" />
          <span>公开页显示电费估算（按下方电价折算）</span>
        </label>
        <label class="field">
          <span class="field-label">电价（元 / kWh）</span>
          <input v-model.number="settings.electric_price" type="number" min="0" max="99" step="0.01" />
        </label>
      </div>
      <p class="sec-hint">功耗读数来自节点 RAPL 功率计（Intel 机型），整机功耗含电池校准的基础功耗，算法与参考实现 power-monitor 一致。</p>
    </div>

    <div class="glass-card settings-card">
      <h4 class="sec-title">公开页面</h4>
      <div class="form-grid">
        <label class="field field-wide">
          <span class="field-label">站点标题</span>
          <input v-model="settings.site_title" type="text" />
        </label>
        <label class="check-field field-wide">
          <input v-model="settings.open_7d_history" type="checkbox" />
          <span>对访客开放 7 天历史曲线（默认仅登录可见）</span>
        </label>
        <label class="check-field field-wide">
          <input v-model="settings.private_mode" type="checkbox" />
          <span>完全私有模式（所有公开 API 要求登录，对应 Nezha 的 force_auth）</span>
        </label>
      </div>
    </div>

    <div class="glass-card settings-card">
      <h4 class="sec-title">SSH 安全</h4>
      <div class="form-grid">
        <label class="check-field field-wide">
          <input v-model="settings.strict_host_key" type="checkbox" />
          <span>Host Key 严格校验（指纹与首次记录不符即拒绝连接；关闭时仅记录 TOFU）</span>
        </label>
      </div>
      <p class="sec-hint">指纹在管理端「节点管理 → 详情」中展示，建议核对后开启严格模式。</p>
    </div>

    <div class="settings-save">
      <button class="btn btn-primary" :disabled="saving" @click="save">
        <AppIcon :name="saved ? 'check' : 'sliders'" class="btn-ico" />
        {{ saved ? '已保存' : saving ? '保存中…' : '保存设置' }}
      </button>
    </div>
  </div>
</template>
