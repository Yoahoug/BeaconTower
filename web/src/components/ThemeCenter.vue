<!-- ============================================================
     主题中心（v2.8 双主题架构，doc/18）：玻璃拟态风主题卡片弹窗
     - 信息结构与 toy 的 ThemeSwitcherModal 对齐（同构 THEME 清单渲染）
     - 入口在侧栏左下状态区，与 toy 侧栏主题坞对位
     ============================================================ -->
<script setup>
import { THEMES, switchTheme } from '../theme'

defineProps({
  open: { type: Boolean, default: false },
})
const emit = defineEmits(['close'])

function onPick(t) {
  if (!t.available || t.current) return
  switchTheme(t)
}
</script>

<template>
  <Transition name="modal">
    <div v-if="open" class="theme-center-scrim" @click.self="emit('close')">
      <div class="bt-card theme-center" role="dialog" aria-modal="true" aria-label="主题中心">
        <header class="theme-center__head">
          <div class="theme-center__title">
            <span class="theme-center__icon" aria-hidden="true">🎨</span>
            <div>
              <h3>视觉风格与主题中心</h3>
              <p>自由切换您喜爱的操作界面风格</p>
            </div>
          </div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" aria-label="关闭" @click="emit('close')">✕</button>
        </header>

        <div class="theme-center__list">
          <template v-for="t in THEMES" :key="t.id">
            <button
              v-if="t.available"
              type="button"
              class="theme-card"
              :class="{ 'is-current': t.current }"
              :aria-current="t.current ? 'true' : undefined"
              @click="onPick(t)"
            >
              <span class="theme-card__icon" aria-hidden="true">{{ t.icon }}</span>
              <span class="theme-card__body">
                <span class="theme-card__name">
                  {{ t.name }}
                  <span class="bt-tag" :class="t.current ? 'bt-tag--success' : ''">{{ t.current ? '当前启用' : '一键切换' }}</span>
                </span>
                <span class="theme-card__desc">{{ t.desc }}</span>
              </span>
            </button>

            <div v-else class="theme-card theme-card--soon" aria-disabled="true">
              <span class="theme-card__icon" aria-hidden="true">{{ t.icon }}</span>
              <span class="theme-card__body">
                <span class="theme-card__name">
                  {{ t.name }}
                  <span class="bt-tag">即将推出</span>
                </span>
                <span class="theme-card__desc">{{ t.desc }}</span>
              </span>
            </div>
          </template>
        </div>

        <footer class="theme-center__foot">
          <span>切换记录已持久化至 LocalStorage</span>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="emit('close')">确定</button>
        </footer>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.theme-center-scrim {
  position: fixed;
  inset: 0;
  z-index: 90;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  background: rgba(30, 41, 59, 0.35);
  backdrop-filter: blur(8px);
}
.theme-center {
  width: min(30rem, 100%);
  max-height: min(80vh, 40rem);
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}
.theme-center__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 1.1rem 1.25rem 0.9rem;
  border-bottom: 1px solid var(--line, rgba(148, 163, 184, 0.25));
}
.theme-center__title {
  display: flex;
  align-items: center;
  gap: 0.65rem;
}
.theme-center__icon {
  font-size: 1.5rem;
}
.theme-center__head h3 {
  margin: 0;
  font-size: 1rem;
  font-weight: 700;
}
.theme-center__head p {
  margin: 0.15rem 0 0;
  font-size: 0.75rem;
  color: var(--text-muted, #64748b);
}
.theme-center__list {
  flex: 1;
  overflow-y: auto;
  padding: 0.9rem 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}
.theme-card {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  width: 100%;
  text-align: left;
  padding: 0.85rem 0.95rem;
  border-radius: 0.9rem;
  border: 1px solid var(--line, rgba(148, 163, 184, 0.3));
  background: rgba(255, 255, 255, 0.55);
  backdrop-filter: blur(10px);
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;
}
.theme-card:hover:not(.is-current):not(.theme-card--soon) {
  background: rgba(255, 255, 255, 0.8);
  transform: translateY(-1px);
  box-shadow: 0 8px 24px rgba(56, 88, 233, 0.14);
}
.theme-card.is-current {
  cursor: default;
  border-color: rgba(56, 189, 248, 0.55);
  background: rgba(224, 247, 254, 0.6);
}
.theme-card--soon {
  opacity: 0.65;
  cursor: default;
  border-style: dashed;
}
.theme-card__icon {
  font-size: 1.35rem;
  line-height: 1;
  padding: 0.45rem;
  border-radius: 0.65rem;
  background: rgba(232, 240, 253, 0.8);
}
.theme-card__body {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  min-width: 0;
}
.theme-card__name {
  font-weight: 700;
  font-size: 0.85rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.theme-card__desc {
  font-size: 0.75rem;
  color: var(--text-muted, #64748b);
}
.theme-center__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.85rem 1.25rem 1.05rem;
  border-top: 1px solid var(--line, rgba(148, 163, 184, 0.25));
  font-size: 0.75rem;
  color: var(--text-muted, #64748b);
}

/* 复用全局 modal 转场（motion.css），无则淡入兜底 */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.18s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
</style>
