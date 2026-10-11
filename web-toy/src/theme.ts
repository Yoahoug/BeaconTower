/**
 * BeaconTower 主题注册清单（doc/18 契约）
 *
 * 未来新增主题三步：
 *   1. 新目录产出 dist（Dockerfile 加阶段、main.go 加 embed 并在 registerThemes 注册）
 *   2. 本文件与 classic 端 web/src/theme.js 各加一条（两份同构，id/path 一致）
 *   3. 后端 router.go registerThemes 加一段（若作为默认主题需改默认位）
 *
 * localStorage 契约：
 *   beacontower_preferred_theme = 'toy' | 'classic'（未来追加同 id）
 *   beacontower_avatar_mode     = 'bot' | 'os'（toy 专用头像偏好，保持兼容）
 */

export interface ThemeEntry {
  id: string;
  name: string;
  desc: string;
  /** 挂载路径：默认主题 '/', 其余 '/<id>/' */
  path: string;
  icon: string;
  /** 是否为当前代码所属主题 */
  current: boolean;
  /** 是否已上线可切换 */
  available: boolean;
}

export const THEMES: ThemeEntry[] = [
  {
    id: 'toy',
    name: 'Toy Neubrutalism · 玩具新野兽派',
    desc: '纯黑物理注塑外框、高对比度积木明黄配色、趣味像素机器人与清晰运维大字。',
    path: '/',
    icon: '🧸',
    current: true,
    available: true,
  },
  {
    id: 'classic',
    name: 'Classic Glass · 经典天空玻璃拟态',
    desc: '经典亮色拟态界面，极光渐变背景与半透明玻璃卡片设计。',
    path: '/classic/',
    icon: '🪟',
    current: false,
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
];

export const THEME_STORAGE_KEY = 'beacontower_preferred_theme';

/** 读取持久化主题偏好（非法值视为未设置） */
export function readPreferredTheme(): string | null {
  try {
    const v = localStorage.getItem(THEME_STORAGE_KEY);
    return v && THEMES.some((t) => t.id === v) ? v : null;
  } catch {
    return null;
  }
}

export function savePreferredTheme(id: string) {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, id);
  } catch {
    /* 隐私模式写失败不阻塞 */
  }
}
