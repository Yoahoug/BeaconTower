<script setup>
import { RouterLink } from 'vue-router'
</script>

<template>
  <main class="page">
    <div class="stub-wrap">
      <div class="stub-card">
        <h2>管理面板 · 规划中</h2>
        <p class="desc">
          管理后台尚未实现，当前为占位页。此页面为隐藏后台，仅通过 <code>/admin</code> 地址访问，不设任何公开入口。
        </p>
        <ul class="plan-list">
          <li>
            <span class="num">1</span>
            <span>
              初始化向导（首次访问）
              <span class="note">首次打开时设置管理员账号与强密码，完成后此入口永久关闭</span>
            </span>
          </li>
          <li>
            <span class="num">2</span>
            <span>
              服务器管理
              <span class="note">添加 / 编辑节点：SSH 地址、端口、认证方式（密码或密钥），支持一键测试连接并回读系统画像；节点地区由公网 IP 自动定位填充，无需手动输入，可随时修正（IP 本身不公开）</span>
            </span>
          </li>
          <li>
            <span class="num">3</span>
            <span>
              采集与展示设置
              <span class="note">采集间隔、历史数据保留策略、公开页面显示字段开关</span>
            </span>
          </li>
          <li>
            <span class="num">4</span>
            <span>
              安全与审计
              <span class="note">SSH 凭据 AES-GCM 加密存储、登录限速、操作审计日志</span>
            </span>
          </li>
        </ul>
        <div class="stub-foot">
          原型阶段此页仅作风格示意。详细接口与字段设计见 <code>doc/04-API接口设计.md</code>。
        </div>
        <RouterLink to="/" class="back-link">← 返回监控面板</RouterLink>
      </div>
    </div>
  </main>
</template>
