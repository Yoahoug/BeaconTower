<!-- ============================================================
     总览页（企业级重构版）
     KPI 条 + 全网趋势（ECharts） + 节点网格（搜索/筛选/空态/骨架）
     ============================================================ -->
<script setup>
import { computed } from 'vue'
import { useMonitorStore } from '../../stores/monitor'
import NodeCard from '../../components/monitor/NodeCard.vue'
import TrendChart from '../../components/charts/TrendChart.vue'
import StateEmpty from '../../components/ui/StateEmpty.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import AppIcon from '../../components/AppIcon.vue'
import { fmtBps, fmtWatts, fmtKwh, fmtCost, agoText } from '../../utils/format'

const monitor = useMonitorStore()

const netSeries = computed(() => [
  { name: '上行', color: '#1d9d4e', data: monitor.netUp, fill: true },
  { name: '下行', color: '#0a64e0', data: monitor.netDown, fill: true },
])

const powerSeries = computed(() => [
  { name: '全网功率', color: '#d97a06', data: monitor.watts, fill: true },
])
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>服务器状态总览</h1>
        <p class="page-head__desc">
          全网 {{ monitor.summary.total }} 个节点 · 数据每 10 秒自动刷新 ·
          更新于 {{ agoText(monitor.secondsSinceUpdate) }}
        </p>
      </div>
    </div>

    <!-- 加载 / 错误态 -->
    <StateSkeleton v-if="monitor.status === 'loading'" :rows="5" />
    <StateError
      v-else-if="monitor.status === 'error'"
      :message="monitor.error || '数据加载失败'"
      @retry="monitor.retry()"
    />

    <template v-else>
      <!-- KPI 条：信息密度优先，无装饰动画 -->
      <section class="kpi-strip tnum" aria-label="全网关键指标">
        <div class="kpi-card">
          <div class="kpi-card__value">{{ monitor.summary.total }}</div>
          <div class="kpi-card__label">服务器总数</div>
        </div>
        <div class="kpi-card" :class="monitor.summary.offline ? 'kpi-card--danger' : 'kpi-card--success'">
          <div class="kpi-card__value">
            {{ monitor.summary.online }}<span class="unit">/ {{ monitor.summary.total }}</span>
          </div>
          <div class="kpi-card__label">
            在线节点
            <span v-if="monitor.summary.offline" class="kpi-card__sub">{{ monitor.summary.offline }} 台离线</span>
          </div>
        </div>
        <div class="kpi-card">
          <div class="kpi-card__value">{{ fmtBps(monitor.summary.upBps) }}</div>
          <div class="kpi-card__label">全网上行</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-card__value">{{ fmtBps(monitor.summary.downBps) }}</div>
          <div class="kpi-card__label">全网下行</div>
        </div>
        <div class="kpi-card kpi-card--warn">
          <div class="kpi-card__value">
            {{ monitor.summary.measuredCount ? fmtWatts(monitor.summary.watts) : '—' }}
          </div>
          <div class="kpi-card__label">
            实时功耗
            <span v-if="monitor.summary.measuredCount" class="kpi-card__sub">{{ monitor.summary.measuredCount }} 台可测</span>
          </div>
        </div>
        <div class="kpi-card">
          <div class="kpi-card__value">
            {{ monitor.summary.measuredCount ? fmtKwh(monitor.summary.monthKwh) : '—' }}
          </div>
          <div class="kpi-card__label">
            本月用电
            <span v-if="monitor.summary.measuredCount" class="kpi-card__sub">≈ {{ fmtCost(monitor.summary.estCostMonth) }}</span>
          </div>
        </div>
      </section>

      <!-- 全网趋势 -->
      <section class="bt-card trend-panel" aria-label="全网趋势">
        <div class="bt-card__head trend-panel__head">
          <div class="bt-card__title">全网吞吐 · 近 2 分钟</div>
          <div class="trend-legend tnum">
            <span><i class="lg up"></i>上行 {{ fmtBps(monitor.summary.upBps) }}</span>
            <span><i class="lg down"></i>下行 {{ fmtBps(monitor.summary.downBps) }}</span>
          </div>
        </div>
        <div class="bt-card__body">
          <TrendChart :series="netSeries" :height="220" label="全网吞吐趋势" :y-formatter="fmtBps" />
          <template v-if="monitor.summary.measuredCount">
            <div class="trend-legend tnum" style="margin: 12px 0 4px">
              <span><i class="lg power"></i>功耗 {{ monitor.summary.watts.toFixed(1) }} W</span>
            </div>
            <TrendChart
              :series="powerSeries"
              :height="120"
              label="全网功耗趋势"
              :y-formatter="(v) => `${Number(v).toFixed(1)} W`"
            />
          </template>
        </div>
      </section>

      <!-- 搜索 + 筛选 -->
      <div class="node-toolbar" role="search">
        <input
          :value="monitor.query"
          type="search"
          class="bt-input node-search"
          placeholder="搜索名称 / 地区 / 标签"
          aria-label="搜索节点"
          @input="monitor.setQuery($event.target.value)"
        />
        <div class="bt-seg" role="group" aria-label="按状态筛选">
          <button
            type="button"
            class="bt-seg__item"
            :aria-pressed="monitor.statusFilter === 'all'"
            @click="monitor.setStatusFilter('all')"
          >
            全部
          </button>
          <button
            type="button"
            class="bt-seg__item"
            :aria-pressed="monitor.statusFilter === 'online'"
            @click="monitor.setStatusFilter('online')"
          >
            在线
          </button>
          <button
            type="button"
            class="bt-seg__item"
            :aria-pressed="monitor.statusFilter === 'offline'"
            @click="monitor.setStatusFilter('offline')"
          >
            离线
          </button>
        </div>
        <span class="bt-text-muted" style="font-size: 12px">共 {{ monitor.filteredServers.length }} 个节点</span>
      </div>

      <!-- 节点网格 -->
      <div v-if="monitor.filteredServers.length" class="node-grid">
        <NodeCard
          v-for="s in monitor.filteredServers"
          :key="s.id"
          :server="s"
          :last-updated="monitor.secondsSinceUpdate"
        />
      </div>
      <div v-else class="bt-card">
        <StateEmpty
          title="没有匹配的节点"
          desc="尝试更换关键词或状态筛选。"
          icon="search"
        >
          <button
            class="bt-btn bt-btn--default bt-btn--sm"
            type="button"
            @click="monitor.setQuery(''); monitor.setStatusFilter('all')"
          >
            <AppIcon name="refresh" aria-hidden="true" />清除筛选
          </button>
        </StateEmpty>
      </div>
    </template>
  </div>
</template>
