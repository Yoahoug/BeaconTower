<!-- ============================================================
     WG 组网状态 · 游客版（v2.4）
     数据源：GET /api/v1/public/wg（字段硬白名单，见 doc/04 §1.6）
     只回答「组网跑没跑、中心健不健康、成员在线几个、今天中转多少」；
     网段 / WG IP / 中心端点端口 / 公钥指纹 / 错误原文 / 设备成员名均不下发，
     公开页不得渲染（响应体禁止串扫描钉死，见 wg_public_test.go）。
     ============================================================ -->
<script setup>
import { computed, onMounted, onUnmounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useMonitorStore } from '../../stores/monitor'
import TweenNumber from '../../components/ui/TweenNumber.vue'
import StateEmpty from '../../components/ui/StateEmpty.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import AppIcon from '../../components/AppIcon.vue'
import { fmtBytes, agoText } from '../../utils/format'

const monitor = useMonitorStore()

const hubs = computed(() => monitor.wg?.hubs || [])
const peers = computed(() => monitor.wg?.peers || [])
const summary = computed(() => monitor.wg?.summary || {})
const initialized = computed(() => !!monitor.wg?.initialized)

// 中心「上次巡检」是巡检 ticker 回写的 unix 秒（5 分钟一轮），文案跟随 agoText
function checkedText(at) {
  const ts = Number(at) || 0
  if (!ts) return '尚未巡检'
  return `巡检于 ${agoText(Math.max(0, Math.floor(Date.now() / 1000) - ts))}`
}

onMounted(() => monitor.startWg())
onUnmounted(() => monitor.stopWg())
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>组网状态</h1>
        <p class="page-head__desc">
          WireGuard 组网健康度与成员在线 · 每 60 秒刷新 ·
          更新于 {{ agoText(monitor.secondsSinceUpdate) }}
        </p>
      </div>
    </div>

    <StateSkeleton v-if="monitor.wgStatus === 'loading'" :rows="5" />
    <div v-else-if="monitor.wgStatus === 'error' && monitor.needLogin" class="bt-card">
      <StateEmpty
        title="需登录查看"
        :desc="monitor.wgError || '私有模式下组网状态仅管理员可见，登录后继续浏览。'"
        icon="layers"
      >
        <RouterLink class="bt-btn bt-btn--primary bt-btn--sm" to="/admin/login">前往登录</RouterLink>
      </StateEmpty>
    </div>
    <StateError
      v-else-if="monitor.wgStatus === 'error'"
      :message="monitor.wgError || '组网数据加载失败'"
      @retry="monitor.retryWg()"
    />

    <div v-else-if="!initialized" class="bt-card">
      <StateEmpty
        title="尚未初始化组网"
        desc="管理员在「管理面板 → WG 组网」完成中心节点配置后，这里会显示组网健康度与成员在线状态。"
        icon="layers"
      />
    </div>

    <template v-else>
      <!-- KPI 玻璃条：与穿透状态页同一套（首屏分层淡入 --i 0~3 + 数字补间） -->
      <section class="kpi-strip tnum" aria-label="组网关键指标">
        <div
          class="kpi-card spot bt-enter"
          style="--i: 0"
          :class="summary.hub_healthy < summary.hub_active ? 'kpi-card--danger' : 'kpi-card--brand'"
          v-spotlight
        >
          <div class="kpi-card__label"><AppIcon name="layers" aria-hidden="true" />中心节点</div>
          <div class="kpi-card__value">
            <TweenNumber :value="summary.hub_healthy || 0" /><span class="unit"> / {{ summary.hub_active || 0 }}</span>
          </div>
          <div class="kpi-card__sub">
            {{ summary.hub_active > 0 && summary.hub_healthy >= summary.hub_active ? '现役与备援均健康' : '存在巡检异常的中心' }}
          </div>
        </div>

        <div
          class="kpi-card spot bt-enter"
          style="--i: 1"
          :class="summary.member_online < summary.member_total ? 'kpi-card--danger' : 'kpi-card--success'"
          v-spotlight
        >
          <div class="kpi-card__label"><AppIcon name="server" aria-hidden="true" />成员在线</div>
          <div class="kpi-card__value">
            <TweenNumber :value="summary.member_online || 0" /><span class="unit"> / {{ summary.member_total || 0 }}</span>
          </div>
          <div class="kpi-card__sub">
            {{ summary.member_total > summary.member_online
              ? `${summary.member_total - summary.member_online} 台离线`
              : '全部在线' }}
          </div>
        </div>

        <div class="kpi-card kpi-card--violet spot bt-enter" style="--i: 2" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="user" aria-hidden="true" />在网设备</div>
          <div class="kpi-card__value">
            <TweenNumber :value="summary.device_total || 0" />
          </div>
          <div class="kpi-card__sub">手机 / 电脑等使用端</div>
        </div>

        <div class="kpi-card kpi-card--warn spot bt-enter" style="--i: 3" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="arrow-up" aria-hidden="true" />今日中转流量</div>
          <div class="kpi-card__value">
            <TweenNumber :value="summary.traffic_today || 0" :format="fmtBytes" />
          </div>
          <div class="kpi-card__sub">本月 {{ fmtBytes(summary.traffic_month || 0) }}</div>
        </div>
      </section>

      <div class="mesh-grid">
        <!-- 成员卡片：一张卡 = 一台网内服务器的在线状态（无 IP / 无握手 / 无流量） -->
        <article class="bt-card bt-enter" style="--i: 4">
          <div class="bt-card__head">
            <div class="bt-card__title mesh-card__title">
              <AppIcon name="server" aria-hidden="true" />
              <span>网内成员</span>
            </div>
            <span class="bt-tag" :class="summary.member_online < summary.member_total ? 'bt-tag--warning' : 'bt-tag--success'">
              {{ summary.member_online || 0 }} / {{ summary.member_total || 0 }} 在线
            </span>
          </div>
          <div class="bt-card__body">
            <ul v-if="peers.length" class="mesh-peers">
              <li v-for="p in peers" :key="p.name" class="mesh-peer" :class="{ 'is-off': !p.online }">
                <span
                  class="pulse-dot"
                  :class="{ 'pulse-dot--still': !p.online }"
                  aria-hidden="true"
                />
                <span class="mesh-peer__name">{{ p.name }}</span>
                <span class="mesh-peer__state" :class="p.online ? 'ok' : 'bt-text-warn'">
                  {{ p.online ? '在线' : '离线' }}
                </span>
              </li>
            </ul>
            <p v-else class="bt-text-muted mesh-peers__none">
              暂无网内服务器成员。
            </p>
          </div>
        </article>

        <!-- 中心卡片：现役/备援健康度 + 月度中转量（无端点 / 无端口 / 无密钥） -->
        <article class="bt-card bt-enter" style="--i: 5">
          <div class="bt-card__head">
            <div class="bt-card__title mesh-card__title">
              <AppIcon name="layers" aria-hidden="true" />
              <span>中心节点</span>
            </div>
            <span class="bt-text-muted">{{ checkedText(summary.checked_at) }}</span>
          </div>
          <div class="bt-card__body">
            <ul v-if="hubs.length" class="mesh-peers">
              <li v-for="h in hubs" :key="h.name" class="mesh-peer" :class="{ 'is-off': !h.healthy }">
                <span
                  class="pulse-dot"
                  :class="{ 'pulse-dot--still': !h.healthy }"
                  aria-hidden="true"
                />
                <span class="mesh-peer__name">
                  {{ h.name }}
                  <span v-if="h.is_active" class="bt-tag bt-tag--info mesh-peer__tag">现役</span>
                  <span v-else class="bt-tag bt-tag--outline mesh-peer__tag">备援</span>
                </span>
                <span class="mesh-peer__state" :class="h.healthy ? 'ok' : 'bt-text-warn'">
                  {{ h.healthy ? '健康' : '异常' }}
                </span>
              </li>
            </ul>
            <p v-else class="bt-text-muted mesh-peers__none">暂无中心节点槽位。</p>

            <div class="mesh-hub-traffic tnum">
              <div class="mesh-hub-traffic__row">
                <span>今日中转</span>
                <strong>{{ fmtBytes(summary.traffic_today || 0) }}</strong>
              </div>
              <div class="mesh-hub-traffic__row">
                <span>本月累计</span>
                <strong>{{ fmtBytes(summary.traffic_month || 0) }}</strong>
              </div>
            </div>
          </div>
        </article>
      </div>
    </template>
  </div>
</template>

<style scoped>
.mesh-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: var(--bt-space-4);
}

.mesh-card__title {
  display: flex;
  align-items: center;
  gap: var(--bt-space-2);
}

.mesh-card__title svg {
  width: 18px;
  height: 18px;
  color: var(--bt-brand-500);
}

/* 成员/中心列表：与穿透状态页 frp-node 同一语法（脉冲点 + 名称 + 状态） */
.mesh-peers {
  display: flex;
  flex-direction: column;
  gap: var(--bt-space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.mesh-peer {
  display: grid;
  grid-template-columns: 8px 1fr auto;
  align-items: center;
  gap: var(--bt-space-3);
  padding: var(--bt-space-2) var(--bt-space-3);
  border: 1.5px solid transparent;
  border-radius: var(--bt-radius-md);
  background: var(--bt-bg-subtle);
  font-size: var(--bt-font-md);
}

/* 离线/异常沿用 NodeCard 的语法：虚线描边 + 降饱和 + 明写文字（不只靠颜色） */
.mesh-peer.is-off {
  border: 1.5px dashed var(--bt-border-strong);
  background: transparent;
}

.mesh-peer.is-off .mesh-peer__name,
.mesh-peer.is-off .mesh-peer__state {
  filter: saturate(0.15);
  opacity: 0.6;
}

.mesh-peer__name {
  display: flex;
  align-items: center;
  gap: var(--bt-space-2);
  font-weight: var(--bt-weight-medium);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mesh-peer__tag {
  flex: none;
}

.mesh-peer__state {
  font-size: var(--bt-font-xs);
  color: var(--bt-text-3);
  white-space: nowrap;
}

.mesh-peer__state.ok {
  color: var(--bt-success-600);
}

.mesh-peers__none {
  margin: var(--bt-space-2) 0 0;
  font-size: var(--bt-font-sm);
}

.mesh-hub-traffic {
  display: flex;
  flex-direction: column;
  gap: var(--bt-space-1);
  margin-top: var(--bt-space-4);
  padding-top: var(--bt-space-4);
  border-top: var(--bt-hairline);
}

.mesh-hub-traffic__row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  font-size: var(--bt-font-sm);
  color: var(--bt-text-3);
}

.mesh-hub-traffic__row strong {
  font-size: var(--bt-font-md);
  font-weight: var(--bt-weight-semibold);
  color: var(--bt-text-1);
}

@media (max-width: 720px) {
  .mesh-grid {
    grid-template-columns: 1fr;
  }
}
</style>
