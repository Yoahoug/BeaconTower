<!-- ============================================================
     穿透状态 · 游客版（v2.1）
     数据源：GET /api/v1/public/frp（字段硬白名单，见 doc/13 §12）
     只回答「今天走了多少流量、隧道通不通、在用的节点健不健康」；
     账号画像、套餐余量、节点域名与隧道端点均不下发，公开页不得渲染。
     ============================================================ -->
<script setup>
import { computed, onMounted, onUnmounted } from 'vue'
import { useMonitorStore } from '../../stores/monitor'
import TweenNumber from '../../components/ui/TweenNumber.vue'
import StateEmpty from '../../components/ui/StateEmpty.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import AppIcon from '../../components/AppIcon.vue'
import { fmtBytes, fmtUptime, agoText } from '../../utils/format'

const monitor = useMonitorStore()

const platforms = computed(() => monitor.frp?.platforms || [])
const summary = computed(() => monitor.frp?.summary || {})
const hasPlatform = computed(() => platforms.value.length > 0)

// 节点负载阈值与 GaugeRing 共用一套（60 提示 / 85 告警）
const LOAD_WARN = 60
const LOAD_DANGER = 85

const KIND_LABEL = { natfrp: 'Sakura', chmlfrp: 'ChmlFrp', cloudflared: 'Cloudflare' }
const KIND_ICON = { natfrp: 'tunnel', chmlfrp: 'globe', cloudflared: 'shield' }

// 连接数口径（conns_src）：platform=平台 API 值；local=面板在 frpc 所在节点
// 数 socket（Sakura 平台 API 无连接数字段，doc/13 §1）；空=无数据（未同步或
// 平台口径不适用——Cloudflare 是边缘转发模型，socket 计数无意义），显示「—」。
const connsSupported = computed(() => platforms.value.some((p) => p.conns_src === 'local' || p.conns_src === 'platform'))

function connsSub() {
  if (!connsSupported.value) return '平台未提供 · 面板不可测'
  return '活跃连接（面板节点实测）'
}

function kindLabel(kind) {
  return KIND_LABEL[kind] || '穿透平台'
}

function loadTone(load) {
  const v = Number(load) || 0
  if (v >= LOAD_DANGER) return 'is-danger'
  if (v >= LOAD_WARN) return 'is-warn'
  return ''
}

function loadPct(load) {
  return Math.min(100, Math.max(0, Math.round(Number(load) || 0)))
}

// 面板侧「上次同步」是 unix 秒；平台数据每 3 分钟回流一次，文案跟随 agoText
function syncedText(at) {
  const ts = Number(at) || 0
  if (!ts) return '尚未同步'
  return `同步于 ${agoText(Math.max(0, Math.floor(Date.now() / 1000) - ts))}`
}

// ChmlFrp 的节点接口不报在线时长（恒 0）→ 不显示，避免出现「在线 0 天」
function uptimeText(sec) {
  const s = Number(sec) || 0
  return s > 0 ? `在线 ${fmtUptime(s / 86400)}` : ''
}

function nodeSub(n) {
  return uptimeText(n.uptime) || n.group || '节点'
}

onMounted(() => monitor.startFrp())
onUnmounted(() => monitor.stopFrp())
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>内网穿透状态</h1>
        <p class="page-head__desc">
          Sakura / ChmlFrp / Cloudflare 隧道运行与在用节点健康度 · 每 60 秒刷新 ·
          更新于 {{ agoText(monitor.secondsSinceUpdate) }}
        </p>
      </div>
    </div>

    <StateSkeleton v-if="monitor.frpStatus === 'loading'" :rows="5" />
    <StateError
      v-else-if="monitor.frpStatus === 'error'"
      :message="monitor.frpError || '穿透数据加载失败'"
      @retry="monitor.retryFrp()"
    />

    <div v-else-if="!hasPlatform" class="bt-card">
      <StateEmpty
        title="尚未接入穿透平台"
        desc="管理员在「管理面板 → 内网穿透」绑定 Sakura、ChmlFrp 或 Cloudflare 账号后，这里会显示隧道与节点状态。"
        icon="tunnel"
      />
    </div>

    <template v-else>
      <!-- KPI 玻璃条：首屏分层淡入（--i 0~3）+ 数字补间 -->
      <section class="kpi-strip tnum" aria-label="穿透关键指标">
        <div class="kpi-card kpi-card--success spot bt-enter" style="--i: 0" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="calendar" aria-hidden="true" />今日流量消耗</div>
          <div class="kpi-card__value">
            <TweenNumber :value="summary.traffic_today || 0" :format="fmtBytes" />
          </div>
          <div class="kpi-card__sub">
            {{ summary.platform_total }} 个平台合计
          </div>
        </div>

        <div
          class="kpi-card spot bt-enter"
          style="--i: 1"
          :class="summary.tunnel_online < summary.tunnel_total ? 'kpi-card--danger' : 'kpi-card--brand'"
          v-spotlight
        >
          <div class="kpi-card__label"><AppIcon name="tunnel" aria-hidden="true" />在线隧道</div>
          <div class="kpi-card__value">
            <TweenNumber :value="summary.tunnel_online || 0" /><span class="unit"> / {{ summary.tunnel_total || 0 }}</span>
          </div>
          <div class="kpi-card__sub">
            {{ summary.tunnel_total > summary.tunnel_online
              ? `${summary.tunnel_total - summary.tunnel_online} 条离线`
              : '全部在线' }}
          </div>
        </div>

        <div
          class="kpi-card spot bt-enter"
          style="--i: 2"
          :class="summary.node_online < summary.node_in_use ? 'kpi-card--danger' : 'kpi-card--violet'"
          v-spotlight
        >
          <div class="kpi-card__label"><AppIcon name="globe" aria-hidden="true" />在用节点</div>
          <div class="kpi-card__value">
            <TweenNumber :value="summary.node_online || 0" /><span class="unit"> / {{ summary.node_in_use || 0 }}</span>
          </div>
          <div class="kpi-card__sub">隧道所落节点</div>
        </div>

        <div class="kpi-card kpi-card--warn spot bt-enter" style="--i: 3" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="activity" aria-hidden="true" />实时连接数</div>
          <div class="kpi-card__value">
            <span v-if="!connsSupported" class="bt-text-muted">—</span>
            <TweenNumber v-else :value="summary.conns || 0" />
          </div>
          <div class="kpi-card__sub">{{ connsSub() }}</div>
        </div>
      </section>

      <!-- 平台卡片：一张卡 = 一个平台的用量与在用节点 -->
      <section class="frp-grid" aria-label="各平台穿透状态">
        <article
          v-for="(p, i) in platforms"
          :key="`${p.kind}-${p.name}`"
          class="bt-card frp-card bt-enter"
          :style="{ '--i': Math.min(i + 4, 11) }"
        >
          <div class="bt-card__head">
            <div class="bt-card__title frp-card__title">
              <AppIcon :name="KIND_ICON[p.kind] || 'tunnel'" aria-hidden="true" />
              <span>{{ p.name }}</span>
            </div>
            <div class="frp-card__tags">
              <span class="bt-tag bt-tag--outline">{{ kindLabel(p.kind) }}</span>
              <span class="bt-tag" :class="p.online ? 'bt-tag--success' : 'bt-tag--warning'">
                <span
                  class="pulse-dot"
                  :class="{ 'pulse-dot--still': !p.online }"
                  aria-hidden="true"
                />
                {{ p.online ? '同步正常' : '同步异常' }}
              </span>
            </div>
          </div>

          <div class="bt-card__body">
            <div class="frp-stats tnum">
              <div class="frp-stat">
                <span class="frp-stat__label">今日流量</span>
                <strong v-if="p.kind === 'cloudflared'" class="frp-stat__value bt-text-muted">—</strong>
                <strong v-else class="frp-stat__value">{{ fmtBytes(p.traffic_today || 0) }}</strong>
              </div>
              <div class="frp-stat">
                <span class="frp-stat__label">在线隧道</span>
                <strong class="frp-stat__value">
                  {{ p.tunnel_online || 0 }}<span class="frp-stat__unit"> / {{ p.tunnel_total || 0 }}</span>
                </strong>
              </div>
              <div class="frp-stat">
                <span class="frp-stat__label">连接数</span>
                <strong v-if="p.conns_src !== 'local' && p.conns_src !== 'platform'" class="frp-stat__value bt-text-muted">—</strong>
                <strong v-else class="frp-stat__value">{{ p.conns || 0 }}</strong>
              </div>
            </div>

            <div class="frp-nodes__head">
              <span>在用节点（{{ (p.nodes || []).length }}）</span>
              <span class="bt-text-muted">{{ syncedText(p.updated_at) }}</span>
            </div>

            <ul v-if="(p.nodes || []).length" class="frp-nodes">
              <li v-for="n in p.nodes" :key="n.name" class="frp-node" :class="{ 'is-off': !n.online }">
                <span
                  class="pulse-dot"
                  :class="{ 'pulse-dot--still': !n.online }"
                  aria-hidden="true"
                />
                <span class="frp-node__name">{{ n.name }}</span>
                <span class="frp-node__meta">
                  <span v-if="!n.online" class="bt-text-warn">离线</span>
                  <template v-else>{{ nodeSub(n) }}</template>
                </span>
                <span
                  class="frp-node__load tnum"
                  :class="loadTone(n.load)"
                  role="img"
                  :aria-label="`节点负载 ${loadPct(n.load)}%`"
                >
                  <span class="frp-node__bar" aria-hidden="true">
                    <i :style="{ width: `${loadPct(n.load)}%` }" />
                  </span>
                  {{ loadPct(n.load) }}%
                </span>
              </li>
            </ul>
            <p v-else class="bt-text-muted frp-nodes__none">
              该平台暂无隧道挂载节点（隧道建好后自动出现）。
            </p>
          </div>
        </article>
      </section>
    </template>
  </div>
</template>

<style scoped>
.frp-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: var(--bt-space-4);
}

.frp-card__title {
  display: flex;
  align-items: center;
  gap: var(--bt-space-2);
}

.frp-card__title svg {
  width: 18px;
  height: 18px;
  color: var(--bt-brand-500);
}

.frp-card__tags {
  display: flex;
  align-items: center;
  gap: var(--bt-space-2);
}

/* 用量三栏：等宽数字对齐，窄屏自动换行 */
.frp-stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--bt-space-4);
  padding-bottom: var(--bt-space-4);
  border-bottom: var(--bt-hairline);
}

.frp-stat {
  display: flex;
  flex-direction: column;
  gap: var(--bt-space-1);
  min-width: 88px;
}

.frp-stat__label {
  font-size: var(--bt-font-xs);
  color: var(--bt-text-3);
}

.frp-stat__value {
  font-size: var(--bt-font-xl);
  font-weight: var(--bt-weight-semibold);
  letter-spacing: -0.01em;
}

.frp-stat__unit {
  font-size: var(--bt-font-sm);
  font-weight: var(--bt-weight-normal);
  color: var(--bt-text-3);
}

.frp-nodes__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--bt-space-3);
  margin: var(--bt-space-4) 0 var(--bt-space-2);
  font-size: var(--bt-font-sm);
  font-weight: var(--bt-weight-medium);
  color: var(--bt-text-2);
}

.frp-nodes {
  display: flex;
  flex-direction: column;
  gap: var(--bt-space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.frp-node {
  display: grid;
  grid-template-columns: 8px 1fr auto auto;
  align-items: center;
  gap: var(--bt-space-3);
  padding: var(--bt-space-2) var(--bt-space-3);
  border: 1.5px solid transparent;
  border-radius: var(--bt-radius-md);
  background: var(--bt-bg-subtle);
  font-size: var(--bt-font-md);
}

/* 离线沿用 NodeCard 的语法：虚线描边 + 降饱和 + 明写「离线」文字（不只靠颜色） */
.frp-node.is-off {
  border: 1.5px dashed var(--bt-border-strong);
  background: transparent;
}

.frp-node.is-off .frp-node__name,
.frp-node.is-off .frp-node__load {
  filter: saturate(0.15);
  opacity: 0.6;
}

.frp-node__name {
  font-weight: var(--bt-weight-medium);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.frp-node__meta {
  font-size: var(--bt-font-xs);
  color: var(--bt-text-3);
  white-space: nowrap;
}

.frp-node__load {
  display: flex;
  align-items: center;
  gap: var(--bt-space-2);
  font-size: var(--bt-font-xs);
  color: var(--bt-text-2);
  white-space: nowrap;
}

.frp-node__bar {
  width: 56px;
  height: 6px;
  border-radius: var(--bt-radius-pill);
  background: var(--bt-bg-hover);
  overflow: hidden;
}

.frp-node__bar i {
  display: block;
  height: 100%;
  border-radius: var(--bt-radius-pill);
  background: var(--bt-brand-500);
  transition: width var(--bt-duration-base) var(--bt-ease-out);
}

.frp-node__load.is-warn .frp-node__bar i {
  background: var(--bt-warning-500);
}

.frp-node__load.is-danger .frp-node__bar i {
  background: var(--bt-danger-500);
}

.frp-node__load.is-danger {
  color: var(--bt-danger-600);
}

.frp-nodes__none {
  margin: var(--bt-space-2) 0 0;
  font-size: var(--bt-font-sm);
}

@media (max-width: 720px) {
  .frp-grid {
    grid-template-columns: 1fr;
  }

  /* 窄屏把负载条压到第二行，避免节点名被挤成省略号 */
  .frp-node {
    grid-template-columns: 8px 1fr auto;
  }

  .frp-node__meta {
    display: none;
  }
}
</style>
