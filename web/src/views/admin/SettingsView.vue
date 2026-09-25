<!-- 采集与展示设置 -->
<script setup>
import { onMounted } from 'vue'
import AppIcon from '../../components/AppIcon.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import { useAdminStore } from '../../stores/admin'
import { useUiStore } from '../../stores/ui'

const admin = useAdminStore()
const ui = useUiStore()

onMounted(() => {
  admin.loadSettings()
})

async function save() {
  if (!admin.settings) return
  if (admin.settings.interval_s < 5) {
    admin.settingsError = '采集间隔不能小于 5 秒'
    return
  }
  await admin.saveSettings()
  if (!admin.settingsError) ui.notify('设置已保存')
}
</script>

<template>
  <div class="settings-stack">
    <StateSkeleton v-if="admin.settingsLoading" :rows="5" />
    <StateError v-else-if="admin.settingsError && !admin.settings" :message="admin.settingsError" @retry="admin.loadSettings()" />
    <template v-else-if="admin.settings">
      <section class="bt-card" aria-labelledby="set-collect">
        <div class="bt-card__head"><div id="set-collect" class="bt-card__title">采集</div></div>
        <div class="bt-card__body">
          <div class="bt-form-grid">
            <label class="bt-field">
              <span class="bt-field__label">采集间隔（秒，≥5）</span>
              <input v-model.number="admin.settings.interval_s" class="bt-input" type="number" min="5" max="300" required />
            </label>
            <label class="bt-field">
              <span class="bt-field__label">历史数据保留（天）</span>
              <input v-model.number="admin.settings.retention_days" class="bt-input" type="number" min="1" max="365" />
            </label>
          </div>
          <p class="sec-hint">面板按此间隔 SSH 拉取一轮指标（含功耗采样），间隔越小曲线越细、目标机开销略增。</p>
        </div>
      </section>

      <section class="bt-card" aria-labelledby="set-power">
        <div class="bt-card__head"><div id="set-power" class="bt-card__title">功耗展示</div></div>
        <div class="bt-card__body">
          <div class="bt-form-grid">
            <label class="bt-check span-2">
              <input v-model="admin.settings.show_power_public" type="checkbox" />
              <span>公开页显示功耗数据（功率 / kWh / 走势；未暴露 RAPL 的机型自动显示"不可用"）</span>
            </label>
            <label class="bt-check span-2">
              <input v-model="admin.settings.show_cost_public" type="checkbox" />
              <span>公开页显示电费估算（按下方电价折算）</span>
            </label>
            <label class="bt-field">
              <span class="bt-field__label">电价（元 / kWh）</span>
              <input v-model.number="admin.settings.electric_price" class="bt-input" type="number" min="0" max="99" step="0.01" />
            </label>
          </div>
          <p class="sec-hint">功耗读数来自节点 RAPL 功率计（Intel 机型），整机功耗含电池校准的基础功耗。</p>
        </div>
      </section>

      <section class="bt-card" aria-labelledby="set-public">
        <div class="bt-card__head"><div id="set-public" class="bt-card__title">公开页面</div></div>
        <div class="bt-card__body">
          <div class="bt-form-grid">
            <label class="bt-field span-2">
              <span class="bt-field__label">站点标题</span>
              <input v-model="admin.settings.site_title" class="bt-input" type="text" />
            </label>
            <label class="bt-check span-2">
              <input v-model="admin.settings.open_7d_history" type="checkbox" />
              <span>对访客开放 7 天历史曲线（默认仅登录可见）</span>
            </label>
            <label class="bt-check span-2">
              <input v-model="admin.settings.private_mode" type="checkbox" />
              <span>完全私有模式（所有公开 API 要求登录）</span>
            </label>
          </div>
        </div>
      </section>

      <section class="bt-card" aria-labelledby="set-ssh">
        <div class="bt-card__head"><div id="set-ssh" class="bt-card__title">SSH 安全</div></div>
        <div class="bt-card__body">
          <label class="bt-check">
            <input v-model="admin.settings.strict_host_key" type="checkbox" />
            <span>Host Key 严格校验（指纹与首次记录不符即拒绝连接；关闭时仅记录 TOFU）</span>
          </label>
          <p class="sec-hint">指纹在「节点管理 → 详情」中展示，建议核对后开启严格模式。</p>
        </div>
      </section>

      <div v-if="admin.settingsError" class="bt-alert bt-alert--error" role="alert">
        <AppIcon name="warn" aria-hidden="true" />{{ admin.settingsError }}
      </div>

      <div class="settings-save no-print">
        <button class="bt-btn bt-btn--primary" type="button" :disabled="admin.settingsSaving" @click="save">
          <AppIcon :name="admin.settingsSaved ? 'check' : 'sliders'" aria-hidden="true" />
          {{ admin.settingsSaved ? '已保存' : admin.settingsSaving ? '保存中…' : '保存设置' }}
        </button>
      </div>
    </template>
  </div>
</template>
