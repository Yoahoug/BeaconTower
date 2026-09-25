<!-- ============================================================
     总览页（v2.0 天空信标）
     KPI 玻璃条（数字补间 + 指针辉光）+ 全网趋势（ECharts）+
     节点玻璃卡网格（stagger 入场 / 搜索 / 筛选 / 三态）
     ============================================================ -->
<script setup>
import { computed } from 'vue'
import { useMonitorStore } from '../../stores/monitor'
import NodeCard from '../../components/monitor/NodeCard.vue'
import TrendChart from '../../components/charts/TrendChart.vue'
import TweenNumber from '../../components/ui/TweenNumber.vue'
import StateEmpty from '../../components/ui/StateEmpty.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import AppIcon from '../../components/AppIcon.vue'
import { fmtBps, fmtWatts, fmtKwh, fmtCost, agoText } from '../../utils/format'

const monitor = useMonitorStore()

const netSeries = computed(() => [
  { name: '上行', color: '#0ea5e9', data: monitor.netUp, fill: true },
  { name: '下行', color: '#8b5cf6', data: monitor.netDown, fill: true },
])

const powerSeries = computed(() => [
  { name: '全网功率', color: '#f59e0b', data: monitor.watts, fill: true },
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
      <!-- KPI 玻璃条：首屏分层淡入（--i 0~5，步进 55ms）+ 数字补间 -->
      <section class="kpi-strip tnum" aria-label="全网关键指标">
        <div class="kpi-card kpi-card--violet spot bt-enter" style="--i: 0" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="layers" aria-hidden="true" />服务器总数</div>
          <div class="kpi-card__value">
            <TweenNumber :value="monitor.summary.total" />
          </div>
          <div class="kpi-card__sub">全部纳管节点</div>
        </div>

        <div
          class="kpi-card spot bt-enter"
          style="--i: 1"
          :class="monitor.summary.offline ? 'kpi-card--danger' : 'kpi-card--success'"
          v-spotlight
        >
          <div class="kpi-card__label"><AppIcon name="globe" aria-hidden="true" />在线节点</div>
          <div class="kpi-card__value">
            <TweenNumber :value="monitor.summary.online" /><span class="unit"> / {{ monitor.summary.total }}</span>
          </div>
          <div class="kpi-card__sub">
            {{ monitor.summary.offline ? `${monitor.summary.offline} 台离线` : '全部在线' }}
          </div>
        </div>

        <div class="kpi-card kpi-card--brand spot bt-enter" style="--i: 2" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="arrow-up" aria-hidden="true" />全网上行</div>
          <div class="kpi-card__value">
            <TweenNumber :value="monitor.summary.upBps" :format="fmtBps" />
          </div>
          <div class="kpi-card__sub">实时出口带宽</div>
        </div>

        <div class="kpi-card kpi-card--violet spot bt-enter" style="--i: 3" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="arrow-down" aria-hidden="true" />全网下行</div>
          <div class="kpi-card__value">
            <TweenNumber :value="monitor.summary.downBps" :format="fmtBps" />
          </div>
          <div class="kpi-card__sub">实时入口带宽</div>
        </div>

        <div class="kpi-card kpi-card--warn spot bt-enter" style="--i: 4" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="sparkles" aria-hidden="true" />实时功耗</div>
          <div class="kpi-card__value">
            <TweenNumber
              v-if="monitor.summary.measuredCount"
              :value="monitor.summary.watts"
              :format="fmtWatts"
            />
            <template v-else>—</template>
          </div>
          <div class="kpi-card__sub">
            {{ monitor.summary.measuredCount ? `${monitor.summary.measuredCount} 台可测` : '暂无可测机型' }}
          </div>
        </div>

        <div class="kpi-card kpi-card--brand spot bt-enter" style="--i: 5" v-spotlight>
          <div class="kpi-card__label"><AppIcon name="calendar" aria-hidden="true" />本月用电</div>
          <div class="kpi-card__value">
            <TweenNumber
              v-if="monitor.summary.measuredCount"
              :value="monitor.summary.monthKwh"
              :format="fmtKwh"
            />
            <template v-else>—</template>
          </div>
          <div class="kpi-card__sub">
            {{ monitor.summary.measuredCount ? `≈ ${fmtCost(monitor.summary.estCostMonth)}` : '暂无可测机型' }}
          </div>
        </div>
      </section>

      <!-- 全网趋势 -->
      <section class="bt-card trend-panel" aria-label="全网趋势">
        <div class="bt-card__head">
          <div class="bt-card__title">全网吞吐 · 近 2 分钟</div>
          <div class="trend-legend tnum">
            <span><i class="lg up" />上行 {{ fmtBps(monitor.summary.upBps) }}</span>
            <span><i class="lg down" />下行 {{ fmtBps(monitor.summary.downBps) }}</span>
          </div>
        </div>
        <div class="bt-card__body">
          <TrendChart :series="netSeries" :height="220" label="全网吞吐趋势" :y-formatter="fmtBps" />
          <template v-if="monitor.summary.measuredCount">
            <div class="trend-legend tnum" style="margin: 12px 0 4px">
              <span><i class="lg power" />功耗 {{ monitor.summary.watts.toFixed(1) }} W</span>
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

      <!-- 节点网格：首屏分层淡入（--i 上限 11，超出截断避免长尾等待） -->
      <div v-if="monitor.filteredServers.length" class="node-grid">
        <NodeCard
          v-for="(s, i) in monitor.filteredServers"
          :key="s.id"
          :server="s"
          :last-updated="monitor.secondsSinceUpdate"
          :style="{ '--i': Math.min(i, 11) }"
          class="bt-enter"
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
