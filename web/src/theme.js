// ============================================================
// BeaconTower · 主题注册清单（doc/18 契约，与 web-toy/src/theme.ts 同构）
//
// 未来新增主题三步：
//   1. 新目录产出 dist（Dockerfile 加阶段、main.go 加 embed 并在 registerThemes 注册）
//   2. 本文件与 toy 端 web-toy/src/theme.ts 各加一条（两份同构，id/path 一致）
//   3. 后端 router.go registerThemes 加一段（若作为默认主题需改默认位）
//
// localStorage 契约：
//   beacontower_preferred_theme = 'toy' | 'classic'
// ============================================================

export const THEMES = [
  {
    id: 'toy',
    name: 'Toy Neubrutalism · 玩具新野兽派',
    desc: '纯黑物理注塑外框、高对比度积木明黄配色、趣味像素机器人与清晰运维大字。',
    path: '/',
    icon: '🧸',
    current: false,
    available: true,
  },
  {
    id: 'classic',
    name: 'Classic Glass · 经典天空玻璃拟态',
    desc: '经典亮色拟态界面，极光渐变背景与半透明玻璃卡片设计。',
    path: '/classic/',
    icon: '🪟',
    current: true,
    available: true,
  },
  {
    id: 'terminal',
    name: 'Retro Terminal · 复古极客终端',
    desc: 'CRT 扫描线、磷光绿字与全键盘驱动操作（制作中）。',
    path: '/terminal/',
    icon: '✨',
    current: false,
    available: false,
  },
]

export const THEME_STORAGE_KEY = 'beacontower_preferred_theme'

export function readPreferredTheme() {
  try {
    const v = localStorage.getItem(THEME_STORAGE_KEY)
    return v && THEMES.some((t) => t.id === v) ? v : null
  } catch {
    return null
  }
}

export function savePreferredTheme(id) {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, id)
  } catch {
    /* 隐私模式写失败不阻塞 */
  }
}

/**
 * 切换主题：写偏好后跳目标挂载路径。
 * dev 跨端口联调（classic 5173 / toy 5174）保留；生产同源直接跳。
 */
export function switchTheme(t) {
  savePreferredTheme(t.id)
  if (window.location.port === '5173' && t.id === 'toy') {
    window.location.href = `http://${window.location.hostname}:5174`
    return
  }
  if (window.location.port === '5174' && t.id === 'classic') {
    window.location.href = `http://${window.location.hostname}:5173`
    return
  }
  window.location.href = t.path
}
