<template>
  <div class="space-y-5 select-none">
    <!-- 顶栏操作区 -->
    <div class="flex items-center justify-between flex-wrap gap-2">
      <div>
        <h2 class="text-lg font-black text-toy-black flex items-center gap-2">
          <span>安全与账号</span>
          <span class="toy-badge bg-toy-yellow text-[10px]">密码学防护</span>
        </h2>
        <p class="text-xs font-bold text-toy-muted">管理员主密码 · 活跃会话控制 · 硬件加密隔离</p>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <!-- 1. 修改管理员密码 -->
      <div class="toy-card p-5 bg-white space-y-4">
        <div class="flex items-center gap-2 border-b-2 border-toy-black pb-2.5">
          <span class="text-base">🔐</span>
          <h3 class="text-sm font-black text-toy-black">修改管理员密码</h3>
        </div>

        <form @submit.prevent="handleSubmitPassword" class="space-y-3.5">
          <div>
            <label class="block text-xs font-black text-toy-black mb-1">当前旧密码</label>
            <input 
              v-model="oldPassword"
              type="password" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
          </div>

          <div>
            <label class="block text-xs font-black text-toy-black mb-1">设置新密码</label>
            <input 
              v-model="newPassword"
              type="password" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
          </div>

          <div>
            <label class="block text-xs font-black text-toy-black mb-1">确认新密码</label>
            <input 
              v-model="confirmPassword"
              type="password" 
              class="w-full px-3 py-1.5 text-xs font-bold bg-toy-bg border-2 border-toy-black rounded-lg focus:outline-none font-mono"
              required
            />
          </div>

          <!-- 密码强度校验条 -->
          <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black space-y-1.5">
            <span class="text-[10px] font-bold text-toy-muted block">密码复杂度建议：</span>
            <div class="flex flex-wrap gap-1.5 text-[10px] font-mono">
              <span 
                class="px-2 py-0.5 rounded border border-toy-black font-bold"
                :class="strength.len ? 'bg-toy-mint text-toy-black' : 'bg-white text-toy-muted'"
              >
                {{ strength.len ? '✔ 至少 10 位' : '至少 10 位' }}
              </span>
              <span 
                class="px-2 py-0.5 rounded border border-toy-black font-bold"
                :class="strength.lower ? 'bg-toy-mint text-toy-black' : 'bg-white text-toy-muted'"
              >
                {{ strength.lower ? '✔ 小写字母' : '小写字母' }}
              </span>
              <span 
                class="px-2 py-0.5 rounded border border-toy-black font-bold"
                :class="strength.upper ? 'bg-toy-mint text-toy-black' : 'bg-white text-toy-muted'"
              >
                {{ strength.upper ? '✔ 大写字母' : '大写字母' }}
              </span>
              <span 
                class="px-2 py-0.5 rounded border border-toy-black font-bold"
                :class="strength.digit ? 'bg-toy-mint text-toy-black' : 'bg-white text-toy-muted'"
              >
                {{ strength.digit ? '✔ 数字' : '数字' }}
              </span>
            </div>
          </div>

          <div v-if="errorMsg" class="p-2 bg-red-100 border border-red-500 rounded-lg text-xs font-bold text-red-700">
            {{ errorMsg }}
          </div>
          <div v-if="successMsg" class="p-2 bg-toy-mint/40 border border-toy-black rounded-lg text-xs font-bold text-toy-black">
            {{ successMsg }}
          </div>

          <div class="pt-1">
            <button 
              type="submit" 
              class="toy-btn px-4 py-2 bg-toy-yellow text-xs font-black rounded-xl cursor-pointer"
            >
              更新管理员密码
            </button>
          </div>
        </form>
      </div>

      <!-- 2. 安全机制与凭据存储架构 -->
      <div class="toy-card p-5 bg-white space-y-4 flex flex-col justify-between">
        <div>
          <div class="flex items-center gap-2 border-b-2 border-toy-black pb-2.5">
            <span class="text-base">🛡️</span>
            <h3 class="text-sm font-black text-toy-black">系统内置安全防御机制</h3>
          </div>

          <div class="mt-3.5 space-y-2.5 text-xs">
            <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
              <span class="font-extrabold text-toy-black block">AES-256-GCM 独立主密钥隔离</span>
              <p class="text-[11px] text-toy-muted font-medium mt-0.5">
                所有服务器 SSH 密码及私钥均采用 AES-GCM 高强度密文存储，主密钥独立于数据库单独存放。
              </p>
            </div>

            <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
              <span class="font-extrabold text-toy-black block">Argon2id 密码哈希</span>
              <p class="text-[11px] text-toy-muted font-medium mt-0.5">
                针对主密码使用最新国际密码学标准 Argon2id 内存强化哈希，抵御 GPU 与彩虹表离线破解。
              </p>
            </div>

            <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
              <span class="font-extrabold text-toy-black block">CSRF 双重提交与严格 Cookie</span>
              <p class="text-[11px] text-toy-muted font-medium mt-0.5">
                HttpOnly 会话 Cookie + SameSite=Strict + 双重提交 CSRF 令牌，抵御跨站请求伪造与 XSS 盗取。
              </p>
            </div>

            <div class="p-2.5 bg-toy-bg rounded-lg border border-toy-black">
              <span class="font-extrabold text-toy-black block">TOFU (首次连接信任) 指纹校验</span>
              <p class="text-[11px] text-toy-muted font-medium mt-0.5">
                首次连接自动记录 Host Key 指纹并固化，若目标节点公钥突变立即阻断连接并报警。
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 3. 活动会话管理 (Active Sessions) -->
    <div class="toy-card p-5 bg-white space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-black text-toy-black flex items-center gap-2">
          <span>活动管理会话 ({{ sessions.length }})</span>
          <span class="toy-badge bg-toy-mint text-[10px]">会话实时监测</span>
        </h3>
        <span class="text-xs font-mono text-toy-muted font-bold">登录失效超时: 7 天</span>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b-2 border-toy-black text-toy-muted font-black text-[11px]">
              <th class="pb-2">设备平台 / 客户端</th>
              <th class="pb-2">登录来源 IP</th>
              <th class="pb-2">登录时间</th>
              <th class="pb-2">状态</th>
              <th class="pb-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-toy-gray font-mono">
            <tr v-for="sess in sessions" :key="sess.id" class="hover:bg-toy-bg/50 transition-colors">
              <td class="py-3 font-bold font-sans text-toy-black">
                <div class="flex items-center gap-2">
                  <span>{{ sess.device.includes('iPhone') ? '📱' : '💻' }}</span>
                  <div>
                    <div>{{ sess.device }}</div>
                    <span class="text-[10px] text-toy-muted">{{ sess.browser }}</span>
                  </div>
                </div>
              </td>
              <td class="py-3 text-toy-black font-extrabold">{{ sess.ip }}</td>
              <td class="py-3 font-sans text-toy-muted">{{ formatTime(sess.login_at) }}</td>
              <td class="py-3">
                <span 
                  class="toy-badge text-[10px]"
                  :class="sess.is_current ? 'bg-toy-yellow text-toy-black' : 'bg-toy-bg text-toy-muted'"
                >
                  {{ sess.is_current ? '当前会话' : '活动中' }}
                </span>
              </td>
              <td class="py-3 text-right font-sans">
                <button 
                  v-if="!sess.is_current"
                  @click="handleRevoke(sess.id)"
                  class="px-2 py-1 rounded text-[10px] font-black border border-toy-black bg-white hover:bg-red-50 text-red-600 transition-colors cursor-pointer"
                >
                  踢出会话
                </button>
                <span v-else class="text-[10px] text-toy-muted font-bold">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { SessionItem } from '../../../types/backend';
import { api } from '../../../services/api';

const oldPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const errorMsg = ref('');
const successMsg = ref('');

const strength = computed(() => {
  const p = newPassword.value;
  return {
    len: p.length >= 10,
    lower: /[a-z]/.test(p),
    upper: /[A-Z]/.test(p),
    digit: /[0-9]/.test(p),
  };
});

async function handleSubmitPassword() {
  errorMsg.value = '';
  successMsg.value = '';

  if (newPassword.value !== confirmPassword.value) {
    errorMsg.value = '两次输入的新密码不一致！';
    return;
  }

  const res = await api.changePassword(oldPassword.value, newPassword.value);
  if (res.success) {
    successMsg.value = '密码已成功更新！请妥善保管。';
    oldPassword.value = '';
    newPassword.value = '';
    confirmPassword.value = '';
  } else {
    errorMsg.value = res.msg || '密码更新失败';
  }
}

// 会话管理
const sessions = ref<SessionItem[]>([]);

async function loadSessions() {
  sessions.value = await api.getSessions();
}

async function handleRevoke(id: string) {
  if (confirm('确认强行踢出该设备的登录会话？')) {
    await api.revokeSession(id);
    await loadSessions();
  }
}

function formatTime(ts: number) {
  const d = new Date(ts * 1000);
  return `${d.getMonth() + 1}月${d.getDate()}日 ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

onMounted(() => {
  loadSessions();
});
</script>
