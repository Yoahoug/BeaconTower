<script setup>
import ServerCard from '../components/ServerCard.vue'
import NetGraph from '../components/NetGraph.vue'
import { useLiveServers } from '../composables/useLiveServers'
import { fmtBps, agoText } from '../utils/format'

const { servers, summary, lastUpdated, netHistory } = useLiveServers()
</script>

<template>
  <main class="page">
    <div class="page-head">
      <h1>服务器状态总览</h1>
    </div>

    <!-- 英雄面板：全网概况 + 吞吐走势 -->
    <section v-tilt class="hero-card" aria-label="全网概况">
      <div class="hero-stats">
        <div class="hero-cell">
          <div class="hero-value">{{ summary.total }}</div>
          <div class="hero-label">服务器总数</div>
        </div>
        <div class="hero-cell">
          <div class="hero-value">
            {{ summary.online }}<span class="hero-dim"> / {{ summary.total }}</span>
          </div>
          <div class="hero-label">
            在线节点
            <span v-if="summary.offline" class="hero-offline">· {{ summary.offline }} 台离线</span>
          </div>
        </div>
        <div class="hero-cell">
          <div class="hero-value">{{ fmtBps(summary.upBps) }}</div>
          <div class="hero-label">全网上行</div>
        </div>
        <div class="hero-cell">
          <div class="hero-value">{{ fmtBps(summary.downBps) }}</div>
          <div class="hero-label">全网下行</div>
        </div>
        <div class="hero-cell hero-cell-right">
          <div class="hero-updated">更新于 {{ agoText(lastUpdated.seconds) }}</div>
        </div>
      </div>
      <NetGraph :up="netHistory.up" :down="netHistory.down" />
    </section>

    <!-- 大卡片：单列（窄屏两列） -->
    <div class="server-grid">
      <ServerCard
        v-for="(s, i) in servers"
        :key="s.id"
        :server="s"
        :index="i"
        :last-updated="lastUpdated.seconds"
      />
    </div>
  </main>
</template>
