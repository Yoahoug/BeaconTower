// 演示用模拟数据 —— 全部为虚构信息，不含任何真实 IP / 主机名。
// 按真实部署规模模拟：3 台节点（面板宿主机 + 2 台远端，含 1 台离线示范）。
// 字段结构即后续后端公开 API 的目标形态（不含 IP、主机名等敏感字段）。
// region 由后端按公网 IP 自动定位填充（regionSource: 'auto'），
// 面板宿主机为手动指定示例（'manual'）；IP 本身不出现在公开接口中。
//
// 功耗字段对齐参考实现 power-monitor（RAPL 功率计方案）：
// - watts: 各域功率（W）；rapl 表示该节点是否暴露 RAPL 接口（部分 AMD/虚拟机没有）
// - powerSource: 'ac' | 'battery'（放电时电池功率即整机真实功耗）
// - baseLoadW / baseLoadSource: 平台基础功耗（电池自动校准或人工设定）
// - energy: kWh 统计（今日/本周/本月，梯形积分）与电费估算
// - tempC / freqMhz: 温度与平均 CPU 频率（RAPL 机型通常可读）

const initialHistory = (base) =>
  Array.from({ length: 40 }, () =>
    Math.min(98, Math.max(2, base + (Math.random() * 18 - 9)))
  )

// 功率走势窗口（与 cpuHistory 同步推进，40 点 ≈ 近 2 分钟）
const initialPowerHistory = (base) =>
  Array.from({ length: 40 }, () =>
    Math.max(0.5, base + (Math.random() * 2 - 1) * (base * 0.25))
  )

const mk = (s) => ({
  cpuHistory: initialHistory(s.metrics.cpu),
  powerHistory: s.power?.enabled ? initialPowerHistory(s.power.watts.total) : [],
  ...s,
})

export const mockServers = [
  mk({
    id: 1,
    name: '本机 · BeaconTower 面板',
    region: '本机部署',
    regionSource: 'manual',
    tags: ['面板', '部署机'],
    online: true,
    profile: { os: 'Ubuntu 24.04', icon: 'linux', arch: 'x86_64', cores: 4, virt: 'KVM' },
    metrics: {
      cpu: 12,
      memUsed: 2.6, memTotal: 8,
      diskUsed: 22.4, diskTotal: 120,
      netUp: 62_000, netDown: 340_000,
      tcp: 86, udp: 12,
      load: [0.24, 0.3, 0.32],
      uptimeDays: 210,
      processes: 142,
    },
    power: {
      enabled: true,
      rapl: true,
      powerSource: 'ac',
      watts: { total: 8.2, cpu: 2.6, core: 0.9, uncore: 0.0, dram: 0.5, psys: 1.5 },
      baseLoadW: 5.6,
      baseLoadSource: 'calibration',
      tempC: 46.5,
      freqMhz: 1700,
      energy: { todayKwh: 0.014, weekKwh: 1.416, monthKwh: 6.062, avgWatts: 16.33, estCostToday: 0.01, estCostMonth: 3.64, pricePerKwh: 0.6 },
    },
  }),
  mk({
    id: 2,
    name: '香港 · 轻量 01',
    region: '香港',
    regionSource: 'auto',
    tags: ['Web', '主力'],
    online: true,
    profile: { os: 'Debian 12', icon: 'linux', arch: 'x86_64', cores: 4, virt: 'KVM' },
    metrics: {
      cpu: 47,
      memUsed: 5.6, memTotal: 8,
      diskUsed: 38.2, diskTotal: 60,
      netUp: 1_850_000, netDown: 4_200_000,
      tcp: 312, udp: 44,
      load: [1.35, 1.18, 1.02],
      uptimeDays: 96,
      processes: 187,
    },
    power: {
      enabled: true,
      rapl: false, // KVM 虚拟机不暴露 RAPL，功耗不可得（卡片上显示"不可用"）
      powerSource: 'ac',
      watts: { total: 0, cpu: 0 },
      tempC: null,
      freqMhz: 2400,
      energy: null,
    },
  }),
  mk({
    id: 3,
    name: '东京 · 中转 02',
    region: '东京',
    regionSource: 'auto',
    tags: ['中转'],
    online: false,
    profile: { os: 'AlmaLinux 9', icon: 'linux', arch: 'x86_64', cores: 2, virt: 'KVM' },
    metrics: {
      cpu: 0,
      memUsed: 0, memTotal: 2,
      diskUsed: 9.1, diskTotal: 40,
      netUp: 0, netDown: 0,
      tcp: 0, udp: 0,
      load: [0, 0, 0],
      uptimeDays: 0,
      processes: 0,
    },
    power: {
      enabled: true,
      rapl: true,
      powerSource: 'ac',
      watts: { total: 0, cpu: 0 },
      tempC: null,
      freqMhz: 0,
      energy: null,
    },
    offlineSince: '2 小时前',
  }),
]
