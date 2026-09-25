# 仿 macOS 风格 Web 实现规范

> 本规范由两部分沉淀而来：
> 1. 对 `https://ios.25pan.com/`（一个 Vue 3 单页的 macOS/iOS 桌面模拟站）的**实测计算样式**；
> 2. 对 Apple 官方资料与开源反向工程项目的交叉验证 —— 因为该站点本身也是 Web 克隆，
>    除玻璃配方外，它的图标轮廓、红绿灯尺寸、Dock 图标大小、菜单栏形态**都与 Apple 实际规格有偏差**。
>    凡两者冲突处，本规范一律采用 Apple 实际值，并在第 13.2 节列出全部差异与出处。
>
> 配套可直接运行的实现见项目根目录 `index.html`（纯前端、零依赖、零 emoji、零外部资源）。

---

## 0. 结论速览

把这个风格做对，只有六件事真正决定成败：

1. **玻璃的"体积感"来自高光，不是模糊。** 核心是 5 层 `inset` 高光描边模拟玻璃倒角边缘，
   外加一层顶部偏左的 specular 径向高光；真实 `backdrop-filter` 只需要 **2px**。
2. **图标轮廓必须是连续曲率（G2），不是圆角矩形。** `border-radius: 22%` 看起来"差不多"，
   但那四个"肩部"上的曲率折点，是不像 macOS 的头号来源。用贝塞尔路径画（见 3.5）。
3. **真实模糊很贵，所以把模糊"预算化"。** 线上站点把壁纸预渲染成一张 base64 JPEG 快照
   （`--desktop-glass-snapshot`，约 160 KB），玻璃元素只对这张已模糊的图做轻微模糊。
   上百个 `backdrop-filter` 的合成开销被压到一次离屏模糊。
4. **尺寸按 1 CSS px ≈ 1 pt 对齐 Apple 实际值。** Dock 图标 48pt、红绿灯 12pt、菜单栏 24pt、
   统一工具栏 52pt。凭感觉放大是"整体偏笨"的根源，而且自己很难察觉。
5. **玻璃的下一步是「折射」而不是「更糊」。** inset 高光 + `backdrop-filter: blur()` 只能做磨砂亚克力；
   macOS 26 的液态玻璃是**透镜**：用形状 SDF 的梯度生成位移贴图，交给 `feDisplacementMap`，
   再叠一层单光源的边缘光。只有 Chromium 支持，但作为渐进增强成本极低（见 3.2）。
6. **层次靠投影和灰度，不靠描边。** 焦点窗口 `0 8px 24px rgba(0,0,0,.42)`，
   非焦点窗口降到 `0 4px 14px rgba(0,0,0,.26)` 并且红绿灯整体变灰。
   所有动效共用一条缓动曲线 `cubic-bezier(.32,.72,0,1)`（先快、后极缓）。

---

## 1. 分层结构

自下而上，固定定位，用 `z-index` 划分职责：

| 层 | z-index | 说明 |
|---|---|---|
| 壁纸（清晰层） | 0 | 渐变 / 图片，承载 `--wallpaper-*` 滤镜 |
| **玻璃快照底座** | 1 | 壁纸的预模糊副本，玻璃元素"透"的就是这一层 |
| 亮度 / 护眼遮罩 | 2 | 独立叠加层，与壁纸解耦（线上站点为单独 DOM 节点） |
| 桌面（图标 + 小组件） | 3 | 可选中、可框选 |
| **窗口容器** `#windows` | 20 | **自身是堆叠上下文**，窗口之间的次序只在这个上下文内排序 |
| 菜单栏 | 200 | 永远在窗口之上 |
| Dock | 210 | |
| Dock 悬浮提示 | 215 | |
| 弹出面板（菜单 / 控制中心） | 220 | |
| 通知 / Toast | 230 | 最高，不被任何东西遮挡 |

**关键点：窗口之所以不会盖住菜单栏，靠的不是窗口自己的 `z-index`，而是容器
`#windows` 的 `position: fixed; z-index: 20` 形成堆叠上下文。** 窗口在其中
随便怎么递增都出不来。这个结构比"给窗口分配 20~90、菜单栏 35"要稳得多 ——
后者一旦窗口数量超过阈值就会翻车。

线上站点的实际层级可作为对照：菜单栏 `z-index: 35`、侧栏在其父窗口内 `z-index: 2`、
玻璃面片作为独立 `<span>` 覆盖在容器内 `z-index: 1`。

**另一个要点：玻璃面片是一个独立的、`pointer-events: none` 的绝对定位子元素**，
盖在容器上但不吃事件。这样交互层与视觉层完全解耦。

---

## 2. 设计令牌

### 2.1 强调色（直接对齐 Apple System Colors）

```
--mac-blue:   #007AFF    --mac-green:  #34C759    --mac-red:    #FF3B30
--mac-orange: #FF9500    --mac-yellow: #FFCC00    --mac-purple: #AF52DE
```

### 2.2 表面色

| 语义 | 深色 | 浅色 |
|---|---|---|
| 一级背景 | `#1a1a1a` | `#ffffff` |
| 二级背景 | `#1e1e1e` | `#ffffff` |
| 三级背景 | `#161616` | `#fafbfc` |
| 主文字 | `#f7f7f9` | `#171c26` |
| 输入框底 | `#2C2C2E` | `#F2F2F7` |
| 侧栏染色 | `rgba(255,255,255,.13)` | `rgba(255,255,255,.65)` |

> 线上站点还维护了 `--dialog-field-bg: rgba(118,118,128,.12)` 这一档"半透明中性填充色"，
> 用于浅色模式下的输入框，比纯灰更贴合玻璃底。

### 2.2.1 窗口的透明度该怎么分配（一个高发错误）

**macOS 里只有侧栏和工具栏讲 vibrancy，内容区是不透明的。**

很多 Web 克隆把整扇窗都做成半透明玻璃（`rgba(...,0.5)` 左右），
结果是窗口和壁纸糊在一起、层次全失 —— 这一条比任何配色细节都更影响"像不像"。

本项目实测可用的分配（深色外观）：

```css
--window-bg:        rgba(30, 30, 34, 0.90);  /* 窗体外壳：接近不透明，只留一点色偏 */
--window-chrome-bg: rgba(46, 46, 51, 0.72);  /* 统一标题栏：半透明，壁纸透一点 */
--sidebar-bg:       rgba(255, 255, 255, 0.09); /* 侧栏：真正的 vibrancy 面板 */
--content-bg:       rgba(24, 24, 28, 0.55);  /* 内容区：叠在窗体之上 */
```

叠加后的效果是：内容区几乎实心、侧栏是"浮在窗口里的一块玻璃"、标题栏介于两者之间。
浅色外观同理（窗体 `0.92`、标题栏 `0.76`、侧栏 `0.6`）。

判断标准很简单：**把窗口拖到壁纸最花的地方，如果内容区里还能清楚看见壁纸的花纹，
就是太透了。** 侧栏能看见纹理是对的，内容区能看见就是错的。

### 2.3 透明度档位

不同组件的半透明度是**分别配置**的，不是一刀切：

```
--window-opacity: 0.6     --sidebar-opacity: 0.95   --dock-opacity: 0.5
--dialog-opacity: 0.9     --menu-opacity:  0.65
```

规律：**Dock 最透（0.5，因为它贴在壁纸上），侧栏最实（0.95，因为里面有文字列表）。**

### 2.4 圆角

| 元素 | 实测圆角 |
|---|---|
| 窗口 | **16px** |
| 侧栏 | 12px |
| 菜单栏 / 状态栏 | 0（通栏） |
| Dock | **20.16px** |
| 桌面文件夹磁贴 | 20.66px |
| 桌面小组件 | 21.04px |
| 弹出面板 | 12 ~ 20px |
| 应用图标（squircle） | **边长 × 22.37%**，且必须是连续曲率路径 —— 见 3.5 |

Dock 与磁贴的圆角出现 `20.16` 这类小数，是因为它们由图标尺寸乘以百分比算出来的
（`calc(var(--desk-visual-icon-size) * var(--desk-icon-squircle-percent) / 100)`），
**跟着图标大小联动**。这点值得抄：图标尺寸变成可配置项时，圆角会自动跟随。

> ⚠️ **必须纠正的一个常见错误**：用 `border-radius: 15%` 或
> `clip-path: inset(0 round 15%)` 去"模拟超椭圆"是**无效的** ——
> 两者产生的都只是普通圆角矩形，直边与圆角相接处曲率是**跳变**的（G1）；
> 而 Apple 的图标轮廓是 **G2 连续**（曲率本身也连续，没有折点）。
> 在 22% 半径下这个差别肉眼可辨：圆角矩形的四个"肩部"能看出转折，Apple 的看不到。
> 正解见 **3.5**，可运行实现见 `js/icons.js` 的 `squirclePath()`。
>
> 顺带纠正另一个流传很广的说法：n=4 的拉梅超椭圆
> （`|x/a|ⁿ + |y/b|ⁿ = 1`）与实际 Apple 遮罩做逐像素比对**仍有残差**，
> 它不是 Apple 使用的形状。见第 13 节的出处。

### 2.5 模糊半径（注意命名陷阱）

线上站点的变量名是 `--blur-window-radius: 40px`、`--blur-sidebar-radius: 40px`、
`--blur-dialog-radius: 20px`、`--blur-menu-radius: 12px`。
**这里的 "radius" 指的是模糊半径，不是圆角半径** —— 容易误读，自己实现时建议改名为 `--blur-window`。

实测侧栏 `backdrop-filter: blur(40px)`，而 Dock 只有 `blur(2px)`。
差异的原因是：侧栏面积大、内容实，需要强模糊掩盖背景；Dock 下方已有快照底座，只需轻微模糊。

---

## 3. 液态玻璃配方（核心）

以下是**线上站点 Dock 玻璃面片的原始配方**（已按可读格式重排）：

```css
.glass-surface {
  position: absolute;
  inset: 0;
  z-index: 1;
  border-radius: inherit;              /* 跟随父容器 */
  background: #ffffff0a;               /* = rgba(255,255,255,0.04) 极低填充 */
  border: 0.5px solid rgba(255, 255, 255, 0.48);
  box-shadow:
    inset  0    1.25px            rgba(255,255,255,.62),  /* 顶部亮边 */
    inset  2px -2px 1px -1px      rgba(255,255,255,.48),  /* 右上斜向高光 */
    inset -2px  2px 1px -1px      rgba(255,255,255,.32),  /* 左下斜向高光 */
    inset  6px -6px 1px -6px      rgba(255,255,255,.22),  /* 大半径柔光 */
    inset -6px  6px 1px -6px      rgba(255,255,255,.20),  /* 大半径柔光 */
    inset  0 0 2px                rgba(0,0,0,.80),        /* 内侧暗压边 */
    0 4px 8px                     rgba(0,0,0,.20);        /* 外投影 */
  backdrop-filter: blur(2px);
  filter: brightness(.9);              /* 整体压暗，让高光更突出 */
  pointer-events: none;
  contain: layout paint style;         /* 隔离重排/重绘 */
}
```

### 逐条拆解（为什么这样写）

- **`background` 只有 4% 白。** 玻璃不是"白色半透明块"，而是几乎透明的载体，观感全靠高光和背后的模糊。
- **5 层 inset 高光**：顶部 1 条硬亮边 + 两对斜向高光。它们共同制造"光线从左上打过来、
  玻璃有厚度"的错觉。这是整个风格里最关键、也最容易被忽略的一段。
- **1 层 inset 暗边 `0 0 2px rgba(0,0,0,.8)`**：在高光内侧压一条暗线，玻璃立刻有了"边缘剖面"。
- **`border: 0.5px`**：亚像素边框。在 2x 屏上是一条极细的亮线，1px 会显得笨重。
- **`filter: brightness(.9)`**：把整个面片压暗 10%。因为高光是加性的，压暗底色能让高光对比更强。
- **`contain: layout paint style`**：玻璃面片是纯装饰，隔离它可以显著减少合成层重算。

### 低填充变体

搜索指示器这类"更实"的控件用 `rgba(255,255,255,.18)`，圆角直接 `999px` 或 `24px`：

```css
.glass-surface--emphasis { background: rgba(255,255,255,.18); }
```

### 镜面高光层（specular）—— inset 高光之外还要补的一层

inset 阴影负责"倒角边缘"，但它画不出**弧面**上的光。再叠一层顶部偏左的径向高光，
玻璃才真正有"厚度 + 曲率"：

```css
.mac-glass::before {
  content: "";
  position: absolute;
  inset: 0;
  border-radius: inherit;        /* ← 宿主自己必须带 border-radius，否则高光是方的 */
  background: radial-gradient(
    ellipse 80% 40% at 30% 0%,
    rgba(255, 255, 255, 0.35) 0%,
    rgba(255, 255, 255, 0) 70%
  );
  pointer-events: none;
  z-index: 0;                    /* 低于内容；伪元素在 DOM 上先于子元素绘制 */
}
```

**关键原则：光必须来自"一个光源"。** 给四条边都加等亮描边，是玻璃质感退化成
塑料贴纸的最主要原因 —— `radial-gradient` 在一处，而不是 `border` 在四周。

### 大面积变体（Dock / 小组件）

上面那套 5 层高光是**为小控件调出来的**。一旦把它铺到 Dock 这种细长条上，
五条高光沿整条边排开，会变成一圈明显的"白描边"，玻璃立刻变成塑料。

大面积时把高光收敛一档，改用投影建立体积：

```css
.mac-glass--surface {
  background: rgba(255, 255, 255, 0.06);
  border: 0.5px solid rgba(255, 255, 255, 0.2);
  box-shadow:
    inset 0  1px  0 rgba(255,255,255,.20),   /* 只留顶部一条亮边 */
    inset 0 -1px  0 rgba(255,255,255,.05),
    inset  1px 0  0 -0.5px rgba(255,255,255,.09),  /* 左右各一条极细侧边 */
    inset -1px 0  0 -0.5px rgba(255,255,255,.09),
    0 4px 14px rgba(0,0,0,.22),              /* 靠投影撑起层次 */
    0 12px 40px rgba(0,0,0,.26);
  backdrop-filter: blur(6px) saturate(150%);
  filter: none;                             /* 大面积不做 brightness 压暗 */
}
```

**规律：元素越大，高光越少、投影越重。** 这条比任何具体数值都有用。

---

## 3.2 真正的折射：液态玻璃的透镜（进阶，决定"像不像 26"）

上面那套 inset 高光 + `backdrop-filter: blur()` 只能做到"磨砂"。它做得再准，
也只是**一块磨砂亚克力**，不是液态玻璃。区别在三件事，缺一个就穿帮：

| # | 要素 | 说明 |
|---|---|---|
| 1 | **refraction 折射** | 背景穿过玻璃时被"折弯"：边缘最强、中心为 0。这是"透镜" |
| 2 | **caustic 焦散** | 曲率最大的那一圈边缘更亮，像水底的光斑 |
| 3 | **specular 边缘光** | 亮度随曲面法线与光源夹角变化，且**只在一个方向** |

### 怎么做：用位移贴图 + feDisplacementMap

SVG 的 `feDisplacementMap` 按一张"位移贴图"把每个像素搬走：

```
R 通道 = X 轴位移，G 通道 = Y 轴位移，128 = 不位移，范围 ±127px
```

所以**关键不是滤镜，而是那张贴图长什么样**。

> ⚠️ **最常见的错误：拿 `feTurbulence` 噪声当位移图。**
> 那是"随机抖动"，不是"光穿过玻璃"。真实玻璃表面是光滑连续的 ——
> 噪声驱动的位移看起来像热浪扭曲，一眼就假。必须用符合物理的透镜剖面。

### 透镜剖面怎么算：用形状的 SDF

位移量在紧贴边界处最大、向内侧平滑衰减到 0；方向沿着边界法线。
这两件事正好是圆角矩形的**有符号距离场（SDF）**能直接给出的：

```js
// 圆角矩形 SDF：内部为负、边界为 0、外部为正
function sdRoundedRect(px, py, w, h, r) {
  const qx = Math.abs(px - w / 2) - (w / 2 - r);
  const qy = Math.abs(py - h / 2) - (h / 2 - r);
  return Math.hypot(Math.max(qx, 0), Math.max(qy, 0)) + Math.min(Math.max(qx, qy), 0) - r;
}

const t   = clamp(-d / bezel, 0, 1);       // 0 贴边界、1 到折射带内缘
const mag = (1 - t) * (1 - t);             // 位移量：边缘最强，向内平滑归零

// 数值梯度 = 外法线（比解析解稳，换成 squircle 也通用）
const nx = sd(p + eps) - sd(p - eps);
const ny = sd(p + eps) - sd(p - eps);
// 位移指向内侧：背景在边缘被"拉进去"，形成收边透镜
```

把 `128 + (-n · mag) * 127` 写进 R/G 通道，就得到一张位移贴图。
`feDisplacementMap` 的 `scale` 直接用"最大位移像素数"。

### 滤镜链

```xml
<filter id="lg" color-interpolation-filters="sRGB" x="0" y="0" width="1" height="1">
  <feImage href="{lens-map}" width="{w}" height="{h}" preserveAspectRatio="none" result="dmap"/>
  <feGaussianBlur in="SourceGraphic" stdDeviation="0.6" result="soft"/>
  <feDisplacementMap in="soft" in2="dmap" scale="{maxDisp}"
                     xChannelSelector="R" yChannelSelector="G" result="refracted"/>
  <feColorMatrix in="refracted" type="saturate" values="1.28" result="vivid"/>
  <feImage href="{rim-map}" width="{w}" height="{h}" preserveAspectRatio="none" result="rim"/>
  <feBlend in="rim" in2="vivid" mode="screen"/>
</filter>
```

```css
@supports (backdrop-filter: url(#x)) {
  .glass { backdrop-filter: url(#lg) blur(1.5px); }
}
```

五个必须知道的坑：

1. **`color-interpolation-filters="sRGB"` 不能省。** 默认是 linearRGB，
   位移值会被 gamma 变换扭曲，折射方向和强度都会错。
2. **`feImage` 不会自适应元素尺寸。** 必须显式给 `width`/`height`，
   并配 `preserveAspectRatio="none"` 拉伸。**贴图是按元素像素尺寸生成的**
   —— 尺寸一变就要重建（这也是为什么需要 `resize` 时重建缓存）。
3. **`backdrop-filter: url()` 目前只有 Chromium 支持。** Safari / Firefox
   直接忽略这条声明，元素自动退回上一条 CSS `backdrop-filter`。
   所以**基线必须先用纯 CSS 做好**，折射只是增强。
4. **位移贴图按 1/2 分辨率生成就够**：剖面是光滑的，降采样无损观感，省一半内存。
5. **滤镜要有界**：`filter` 的 `x/y/width/height` 默认是 `-10% ~ 120%`，
   收到 `0/0/1/1`（objectBoundingBox）可以省掉边缘那圈无用的合成。

### 边缘光（specular）

同一套 SDF 梯度还能算边缘光，亮度 = **边缘衰减 × max(0, 法线·光源)^n**：

```js
const facing = Math.max(0, nx * LIGHT.x + ny * LIGHT.y);  // 朝光源的外法线才有高光
v = Math.exp(-u * u * 1.35) * Math.pow(facing, 1.7);      // 高斯贴边 + 角度锐化
```

> **单光源是质感的关键。** 给四条边加等亮描边，玻璃立刻变成塑料贴纸；
> 只让"朝光那一侧"亮起来，它才像有厚度的实体。本项目用左上光源
> `(-0.48, -0.88)`，于是上边缘最亮、左下中等、右下几乎无 —— 与 Apple 一致。

### 和"磨砂"的矛盾：折射要求背景有结构

这是实践中最反直觉的一条：

> **折射作用在背景上，背景没有结构，折射就等于不存在。**

而预模糊的"玻璃快照底座"（第 4 节）恰恰会把结构抹平。
所以在开启折射时，本项目让底座层让位给**清晰壁纸**，磨砂改由滤镜链里
那 1~2px 的 `blur()` 提供。这也正是 Apple 的取法 ——
液态玻璃比老版毛玻璃**更清、更透、更会折光**，而不是更糊。

顺带一个推论：**壁纸本身必须有结构**。如果壁纸是一团柔和的渐变，
上面所有玻璃都会退回"磨砂色块"，因为没东西可折。
本项目因此把壁纸改成了双层：背景柔光团（重模糊）+ 前景流带（只 5px 模糊，
保住边缘结构）。这一条直接决定了第 3.2 节的效果能不能被看见。

### 性能实测

担心 `backdrop-filter` 里的 SVG 滤镜很贵是对的，但**贵的是 `feTurbulence`
这类逐像素生成噪声的滤镜**。位移贴图是预渲染好的图片，`feDisplacementMap`
只做一次采样搬移，成本低一个量级。

本项目在 1280×720、4 个窗口打开、Dock 持续来回放大（最坏情况）下实测：

| | 帧间隔中位数 | p95 | p99 | 最差 | >20ms 的帧 |
|---|---|---|---|---|---|
| 开折射 | 5.6ms | 5.7ms | 5.7ms | 5.8ms | **0** |
| 关折射 | 5.6ms | 5.7ms | 5.7ms | 5.7ms | **0** |

稳态帧时**完全一致**；唯一代价是首次合成时的一次性 ~40ms（滤镜编译）。
前提是**折射面积要小**：只给 Dock、小组件、菜单这些"壳层"用，
不要给整扇窗口用。大面上用 CSS `blur()` 就够了。

---

## 3.5 图标体系（决定"像不像"的第一因素）

**先说结论：一个仿 macOS 界面里，只要出现一个 emoji 图标，整体就废了。**
emoji 是彩色位图、由系统字体渲染、形状不可控、跨平台不一致 —— 它和 macOS 图标的
"统一几何 + 受控渐变 + 连续曲率轮廓"是两套完全不同的视觉语言。

### 应用图标规格

```
画布      64 × 64（矢量，任意尺寸清晰）
轮廓      连续曲率圆角（continuous rounded rectangle），边长 × 22.37%
          转角半径 ≈ 14.32（在 64 画布上），corner smoothing = 0.6
          —— 必须是贝塞尔路径，不能用 <rect rx> / border-radius，原因见下
投影      用 filter: drop-shadow()，不要用 box-shadow
          （图标四角是透明的，box-shadow 会画出一个方影子）
```

### 连续曲率圆角（squircle）到底怎么画

这是整个图标体系里**最容易被做错、也最暴露"仿制感"的一处**。

Apple 的图标轮廓由 **4 段直边 + 4 个转角**组成，每个转角 =
**2 段三次贝塞尔 + 1 段圆弧 + 2 段三次贝塞尔**，一共 5 段。
关键在于：这些贝塞尔段的控制点比例让**曲率**在直边（曲率 0）与圆弧（曲率 1/r）
之间平滑过渡 —— 也就是 G2 连续。`border-radius` 只有 G1 连续，曲率在接点处直接跳变。

可复现的参数（经反向工程验证，与 Apple 遮罩在显示分辨率下视觉一致）：

```
cornerRadius    = 边长 × 22.37%     （iOS 遮罩实测；macOS 官方网格约 22.5%）
cornerSmoothing = 0.6               （Figma 的 iOS 预设值）
```

推导过程（`cornerSmoothing` 记作 s，`cornerRadius` 记作 r，`budget = min(w,h)/2`）：

```js
p        = (1 + s) * r                      // 转角在边上占用的总长度
arcMeasure     = 90 * (1 - s)               // 其中真正是圆弧的角度
arcLen         = sin(arcMeasure/2) * r * √2 // 圆弧段的弦长
p3ToP4Distance = r * tan((90 - arcMeasure) / 4)
beta     = 45 * s
c        = p3ToP4Distance * cos(beta)
d        = c * tan(beta)
b        = (p - arcLen - c - d) / 3
a        = 2 * b
```

`s = 0.6` 时：`arcMeasure = 36°`（圆弧只占 36°，远小于圆角矩形的 90°），
`a = 8.0182, b = 4.0091, c = 3.0625, d = 1.5604, arcLen = 6.2567, r = 14.3168`（w=h=64）。
把 `arcMeasure` 从 90° 收到 36°，正是"去折点"的来源。

算法出处是 **figma-squircle**（MIT），完整可运行实现见 `js/icons.js` 的
`squirclePath(w, h, radiusRatio, smoothing)` / `clipSquircle(el)`。

### 落地方式

每个图标是一段自包含的内联 SVG，用统一外壳包住并自动裁剪。
外壳直接内联算好的 64×64 squircle 路径：

```js
const SQUIRCLE_64 = squirclePath(64, 64);   // 只算一次

const S = (key, defs, body) => `<svg viewBox="0 0 64 64" class="app-icon"><defs>
  <clipPath id="${key}-c"><path d="${SQUIRCLE_64}"/></clipPath>${defs}</defs>
  <g clip-path="url(#${key}-c)">${body}</g></svg>`;
```

要点：

1. **`clipPath` 放在外壳里统一加**，图标内部就可以放心把形状画到边缘，
   不必每个图标自己算圆角裁剪。
2. **渐变 / 滤镜 id 必须按图标命名空间化**（`finA`、`tmA`、`stA`），
   因为内联 SVG 共享同一个 `document`，id 冲突会串色。
3. **尺寸交给容器决定**：`.app-icon { width:100%; height:100% }`，
   外面套一个定尺寸的 `<span>`。同一枚图标在 Dock（48px）、桌面（56px）、
   Dock 悬浮放大（72px）都不用改代码 —— 路径是矢量的，缩放不失真。
4. **几何要能"读出身份"**：访达是蓝底 + 白/浅蓝双色脸 + 两只眼睛 + 一条笑弧；
   终端是深灰底 + 标题栏三个圆点 + `>_`；系统设置是灰底 + 齿轮（齿由循环生成）。
   这些特征在 48px 下必须仍然可辨 —— 这是唯一的验收标准。
5. **需要展现"当前状态"的图标就做成函数**：日历图标的月份/日期、时钟图标的指针
   都取 `new Date()`，Dock 里的日历永远显示今天。成本极低，真实感提升明显。

### 材质层：让一堆图标变成一套图标

单看一枚图标几乎看不出，但整排 Dock 摆在一起时，差别很明显 ——
Apple 的图标之所以"成套"，是因为**它们有统一的受光方式**。

在外壳的裁剪组里，紧跟图标内容再叠一层材质渐变即可（本项目做法）：

```js
const S = (key, defs, body) => `<svg viewBox="0 0 64 64" class="app-icon"><defs>
  <clipPath id="${key}-c"><path d="${SQUIRCLE_64}"/></clipPath>
  <linearGradient id="${key}-m" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0"   stop-color="#ffffff" stop-opacity=".2"/>
    <stop offset=".44" stop-color="#ffffff" stop-opacity="0"/>
    <stop offset=".76" stop-color="#000000" stop-opacity="0"/>
    <stop offset="1"   stop-color="#000000" stop-opacity=".14"/>
  </linearGradient>${defs}</defs>
  <g clip-path="url(#${key}-c)">${body}
    <rect width="64" height="64" fill="url(#${key}-m)"/>
  </g></svg>`;
```

一条渐变同时给出"顶部受光 + 底部沉降"，于是蓝色访达、白色备忘录、
深色终端自动获得同一种体积感，不需要逐枚图标手调。

> ⚠️ 注意与图标自身的渐变别打架：材质层要**很轻**（20% / 14%），
> 它的作用是统一，不是塑造。超过这个量就会把浅色图标（备忘录、启动台）搞脏。

### CSS 侧元素怎么用

`clip-path: path()` 需要绝对像素坐标，所以给非 SVG 元素（磁贴底、缩略图框）
提供一个 JS 助手，`boot()` 会扫描 `.mac-squircle` 自动应用并在尺寸变化时重算：

```html
<div class="mac-squircle" style="width:96px;height:96px"></div>
```

```js
MacIcons.clipSquircle(el);   // 读取实际尺寸并写入 clip-path: path(...)
```

> ⚠️ `clip-path` 会把元素**自身的投影一起裁掉**，所以这个类只用在无投影的元素上；
> 带投影的容器请沿用 `border-radius`（窗口、气泡），或者用 SVG 外壳（图标）。

### 界面符号（侧栏 / 工具栏）

侧栏和工具栏**不能复用应用图标**（彩色、48px、太抢眼），要单独做一套 16×16 线性符号：

```
画布      16 × 16
描边      stroke-width 1.4，stroke: currentColor，fill: none
端点      stroke-linecap/linejoin: round
实心部件  单独在元素上写 fill="currentColor" stroke="none"
```

`stroke: currentColor` 是关键：同一个"齿轮"符号放在侧栏（继承文字色）、
选中项（继承白色）、工具栏（继承主题色）都不用改一行代码。

> 本项目共 19 枚应用图标 + 56 枚界面符号，全部手写内联 SVG，约 400 行。
> **零 emoji、零外部资源** —— 这条是硬约束，一个 emoji 就能毁掉整体观感。

---

## 4. 性能关键：玻璃快照（Glass Snapshot）

这是整个方案里**最有工程价值的一条**。

### 问题

一个 macOS 桌面有：11 个 Dock 图标背景 + 4 个小组件 + 十几个桌面磁贴 + 菜单栏 +
若干窗口和侧栏。如果每个都用真实的 `backdrop-filter: blur(40px)`，
浏览器要为每个元素分配合成层并执行离屏模糊，滚动/动画时帧率会崩。

### 线上站点的解法

1. 页面加载时把壁纸渲染成一张**已经模糊好的图片**，转成 base64 JPEG。
2. 写进 CSS 变量：`--desktop-glass-snapshot: url("data:image/jpeg;base64,…")`（实测约 **160 KB**）。
3. 在 `<html>` 上打标记类 `desktop-glass-snapshot-ready`。
4. 玻璃元素背后垫这一层快照，于是**真实 `backdrop-filter` 只需要 2~3px**（实测 Dock 为 `blur(2px)`、
   文件夹磁贴为 `blur(3px)`）。

代价是一次离屏模糊 + 160 KB 内存，收益是上百个玻璃元素几乎零成本。

### 本项目的实现：canvas 管线 + 极小底座

壁纸是程序化生成的 SVG（深色底 + 柔光块 + `feTurbulence` 位移流带 + 颗粒 + 暗角）。
直接把它当 `background-image` 用会**卡死**，原因值得记住：

> **SVG 滤镜是「每像素」成本，且以元素的实际渲染尺寸为基准。**
> `feTurbulence` 叠在满屏元素上，等于对 1440×860 个像素逐个跑噪声函数，
> 再叠 `feDisplacementMap` 和 `feGaussianBlur`。清晰层 + 底座层 = 四遍满屏噪声，
> 实测直接把渲染/截图的合成拖到 3 秒超时。

正确做法就是原站那条路 —— **光栅化一次，之后只用位图**：

```js
// SVG → Image → canvas → JPEG（每套配色只做一次，进缓存）
const img = new Image();
img.onload = () => {
  ctx.drawImage(img, 0, 0, W, H);
  const url = cv.toDataURL("image/jpeg", 0.86);
};
img.src = svgDataUri;
```

然后在 canvas 这一步顺手多做一件事：**再产出一张极小尺寸的玻璃底座**。

```js
const sharp = draw(img, 1024, 640, 0.86);   // 清晰层   ≈ 42 KB
const mesh  = draw(img,  120,  76, 0.80);   // 玻璃底座 ≈ 2.8 KB
```

底座故意做到 120×76。它被 CSS 放大到全屏时，**双线性插值本身就是一次模糊** ——
于是完全不需要 `filter: blur(48px)`，省掉了满屏模糊的合成开销：

```css
.wallpaper-mesh {
  background-size: cover;   /* 120×76 被拉伸到全屏，天然平滑 */
  /* 没有 filter —— 这就是省下来的钱 */
}
```

**这是整个方案里性价比最高的优化：玻璃底座只需要「低分辨率的颜色场」，
根本不需要真实的模糊滤镜。** 顺带还消掉了"边缘露白"问题 ——
不再有滤镜向外衰减像素，也就不需要 `scale(1.12)` 去补偿。

配套地，玻璃表面的实时 `backdrop-filter` 也可以压得很低：

| 层 | backdrop-filter | 理由 |
|---|---|---|
| 大面积（Dock / 小组件） | `blur(6px) saturate(150%)` | 底座已经平滑，只需一点磨砂感 |
| 窗口 / 侧栏 | `blur(2px)` | 同上 |
| 弹出面板 | `blur(24px)` | 面积小且瞬态，可以给足 |

演示里提供了"玻璃快照"开关（系统设置 / 控制中心 / 右键菜单）：
关掉就隐藏底座、每个玻璃元素改做 `blur(30px)` 真模糊，可直接对比手感与开销。

> 壁纸若是用户上传的照片，同一套管线照用：`ctx.filter = 'blur(60px)'` 画一次得到底座即可。
> 跨域图片记得 `crossOrigin="anonymous"`，否则 canvas 被污染、`toDataURL` 抛异常
> （本项目对这一步做了失败回退，退回 SVG 直出）。

### 与折射的冲突：两者互斥

第 3.2 节的折射和这一节的快照底座**互相排斥**，因为诉求正好相反：

| | 快照底座 | 折射 |
|---|---|---|
| 需要的背景 | **已模糊的**（越平滑越省） | **有结构的**（越清晰越明显） |
| 磨砂从哪来 | 底座本身 | 滤镜链里的 `blur(1~2px)` |
| 成本 | 一次离屏模糊，之后全免费 | 每个元素一次 `feDisplacementMap` |

所以本项目把它们做成**两个独立开关**，按场景二选一：

- **要性能 / 要老版毛玻璃观感** → 开快照底座，关折射。
- **要 macOS 26 的液态玻璃** → 开折射（自动隐藏底座），磨砂交给那 1~2px。

实测折射的开销只在**首次合成**（一次性 ~40ms），
稳态帧时间与关闭时完全一致，所以本项目的默认是**开折射**。

---

## 5. 窗口外壳规范

### 5.1 实测几何

线上站点在 1280×720 视口下：

```
窗口整体  1120 × 672 @ (80, 24)
圆角      16px
投影      0 8px 24px rgba(0,0,0,.42)
溢出      overflow: hidden
侧栏      200 × 656，位于窗口内边距 8px 处，圆角 12px
          background rgba(255,255,255,.19)
          backdrop-filter blur(40px)
          box-shadow 0 2px 16px rgba(0,0,0,.3)
红绿灯    直径 12pt，间隙 8pt（中心距 20pt）
          窗口左边到首个按钮圆心 20pt
          填充 #ed6a5f / #f6be50 / #61c555，描边 #e24b41 / #e1a73e / #2dac2f
```

注意**侧栏比窗口边缘内缩 8px**，自身还带圆角和投影 —— 这是 macOS 近两代的设计语言：
侧栏是"浮在窗口里的一块玻璃"，而不是贴着窗口边的实心栏。抄这个细节收益很大。

### 5.2 统一标题栏

红绿灯与工具栏在**同一行**（Big Sur 之后的形态），不是上下两行：

```
[● ● ●]  ‹ ›    窗口标题（居中）            [分段控件] ⇪ ⋯ ⌕
```

- 标题绝对居中，左右两侧用 `flex: 1` 的占位保证居中不偏移。
- 带标签的统一工具栏高度约 **52pt**；纯图标工具栏约 34~38pt。
- 红绿灯垂直居中于标题栏，圆心距左边 20pt（= 按钮左边缘内缩 14px）。
  **不要给容器加左 padding 再加 `margin-left`**，两处叠加会让按钮跑偏；
  让容器左 padding 为 0，由 `.traffic` 自己带内缩，几何才可预测。

### 5.3 红绿灯行为（最容易做错的一组细节）

线上站点那一版是 16px 圆 / 24px 轴距、且只用一个半透明白当"灰色"，
这是**浏览器克隆最显眼的破绽之一**。真实规格与配色：

```css
:root {
  --tl-size: 12px;   /* 直径，Apple 是 12pt */
  --tl-gap: 8px;     /* 间隙（不是中心距），即中心距 20px */
  --tl-inset: 14px;  /* 窗口左边 → 首个按钮左边缘 */
  --tl-close: #ed6a5f;  --tl-close-edge: #e24b41;
  --tl-min:   #f6be50;  --tl-min-edge:   #e1a73e;
  --tl-zoom:  #61c555;  --tl-zoom-edge:  #2dac2f;
  --tl-idle: #4b4b4d;   --tl-idle-edge:  #5a5a5c;   /* 深色外观 */
}
.traffic { display: flex; gap: var(--tl-gap); margin-left: var(--tl-inset); }

/* 非焦点：统一灰填充 + 灰描边 */
.traffic__btn {
  width: var(--tl-size); height: var(--tl-size);
  border-radius: 50%;
  background: var(--tl-idle);
  box-shadow: inset 0 0 0 .5px var(--tl-idle-edge);
}
/* 焦点：彩色填充 + 同色系深一档描边 */
.window.active .traffic__btn--close {
  --tl-color: var(--tl-close); --tl-edge: var(--tl-close-edge); --tl-glyph: #460804;
}
.window.active .traffic__btn {
  background: var(--tl-color);
  box-shadow: inset 0 0 0 .5px var(--tl-edge);
}
/* 悬浮时三个一起上色（hover 挂在容器上，不是逐个按钮） */
.window.active .traffic:hover .traffic__btn {
  background: var(--tl-color);
  box-shadow: inset 0 0 0 .5px var(--tl-edge);
}
```

三条踩坑记录：

1. **描边不能省**。`box-shadow: inset 0 0 0 .5px` 这半像素边是立体感的来源；
   纯色圆片看起来像贴纸。`border` 会在 12px 上挤掉内容，用 inset shadow 更好控。
2. **"灰色"要用真灰色，不是半透明白**。`rgba(255,255,255,.28)` 在深色窗口上
   会偏亮偏冷，而 Apple 的非焦点态是**中性的深灰填充 + 略亮的灰描边**。
   浅色外观下才是 `#dddddd` / `#d1d0d2`。
3. **符号只在鼠标进入"整组"时淡入**，且符号颜色是深棕/深绿系
   （`#460804` / `#90591d` / `#2a6218`），不是纯黑。
   写 `.traffic:hover .traffic__btn svg { opacity: 1 }`，hover 挂容器。

### 5.4 焦点态

| 状态 | 投影 |
|---|---|
| 焦点窗口 | `0 8px 24px rgba(0,0,0,.42)` + 一圈 `0 0 0 .5px rgba(255,255,255,.16)` 极细高光 |
| 非焦点窗口 | `0 4px 14px rgba(0,0,0,.26)` |

**层次完全靠投影强度表达，不靠边框颜色。** 这是 macOS 与多数 Web 克隆最大的差别。

### 5.5 窗口底部状态栏

高度 26px，左侧一句描述、右侧一句计数（如"12 个项目" / "可用 245.31 GB"），
`border-top: 1px solid var(--divider)`。加了它窗口立刻"像原生应用"。

---

## 6. 菜单栏 / 状态栏规范

两种形态的"顶栏"完全不同：

### 6.1 macOS 形态：菜单栏

```
高度      24px（Apple 实测 24pt）
字号      13px / 字重 500
内边距    0 14px
z-index   200（永远在窗口之上）
```

### 6.2 iPadOS 形态：状态栏 + 灵动岛（本项目当前采用）

对 `ios.25pan.com` 的实测 —— 顶栏是**三段式**：左侧苹果菜单与 App 菜单、
**正中一块灵动岛**、右侧系统状态。灵动岛是覆盖在状态栏之上的独立层级。

**状态栏（`.desk-status-bar--pc`）**：

```
高度      28px，全透明，无 blur
文字      13px / 500，rgba(255,255,255,.92)
内边距    0 16px
左段      苹果 logo(28px 方形按钮) + App 名 + 菜单项，从 x=16 起
右段      Wi-Fi / 电池(25×13，充电时绿色) / 搜索 / 控制中心 / "9月20日周日 21:26"
电池      外框 25×13 圆角 3.4，内芯 21×9
```

**灵动岛（`.di-desk`）**：

```
收起      196 × 34px，水平居中，顶部贴边（top: 0）
          纯黑 #000，圆角 0 0 22px 22px（只有下侧两角）
          内容：时间 + 星期 + 分隔线 + 天气图标 + 温度，13px/500
展开      580 × 118px，同一圆角
          内部是多页横向轨道（概览/天气/闹钟/世界时钟/音乐），可翻页
过渡      width/height/border-radius 0.68s cubic-bezier(.22,1,.36,1)
```

要点：

1. **只有下侧两角是圆的** —— 灵动岛从屏幕顶边"垂下来"，上边没有缝。
   四角全圆是常见错误。
2. 收起态是**纯黑实心**，不是玻璃：岛在壁纸上必须读作"一个洞"，
   加模糊或透明白立即破功。
3. 展开/收起用**同一元素改 width/height**（不是 scale），内容层交叉淡入淡出 ——
   这样圆角在过渡中始终正确。
4. 状态栏字色同样按壁纸亮度自适应（复用 macOS 形态的 `data-menubar-fg` 机制）。
5. 另有整屏对比层（`.desk-root__scrim`）：
   `radial-gradient(circle at 50% 0%, white/12%, transparent 42%)` +
   `rgba(0,0,0,.16)` 打底 —— 顶部中央给一圈白辉光、整屏压暗 16%，
   是参考站点保证顶部白色文字可读的手段（见 `wallpaper-scrim`）。

### 6.3 macOS 26 Tahoe 的菜单栏是"透明"的

这是近两代最大的视觉变化，也是"看起来像不像 26"的分水岭：
  （老版本那种 `backdrop-filter: blur()` 的灰色横条已经取消了）。
- **文字颜色自适应**：系统按壁纸亮度在白色/黑色之间切换，保证可读性。
- 菜单**下拉**面板保留玻璃材质（这是为了可读性，不是遗漏）。
- 用户可以在系统设置里恢复"显示菜单栏背景"。

网页版的落地方式（本项目实现）：

```css
.menubar {
  /* 不做 backdrop-filter；只加一层极淡的顶部渐变压暗兜底 */
  background: var(--menubar-bg);
  text-shadow: var(--menubar-shadow);
}
/* 由 wallpaper.js 采样壁纸顶部亮度后写到 <html data-menubar-fg> */
:root[data-menubar-fg="light"] .menubar {
  --menubar-fg: rgba(255,255,255,.94);
  --menubar-bg: linear-gradient(rgba(0,0,0,.22), rgba(0,0,0,0));
  --menubar-shadow: 0 1px 2px rgba(0,0,0,.45);
}
:root[data-menubar-fg="dark"] .menubar {
  --menubar-fg: rgba(0,0,0,.88);
  --menubar-bg: linear-gradient(rgba(255,255,255,.32), rgba(255,255,255,0));
  --menubar-shadow: 0 1px 2px rgba(255,255,255,.5);
}
```

亮度采样：把壁纸光栅化后的位图顶部约 3.5%（菜单栏在 1600×1000 画面上差不多就是这个比例）
画到 64×4 的 canvas 上，按 Rec.709 求平均亮度，`> 0.55` 判为亮壁纸：

```js
luma += (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
document.documentElement.dataset.menubarFg = luma > 0.55 ? "dark" : "light";
```

> 注意区分：**主题（浅色/深色外观）与菜单栏字色不是一回事**。
> 菜单栏字色取决于*其下方壁纸的亮度*，所以浅色外观 + 暗壁纸仍应显示白字。
> 本项目四种配色的顶部都是深色，因此恒为白字 —— 逻辑仍然要写，
> 一旦加入亮色壁纸它就会立刻起作用。

### 下拉菜单：图标列（Tahoe 新增）

macOS 26 起，菜单项**左侧统一有 SF Symbol 图标**，形成一条可扫读的竖列；
右键上下文菜单同样遵循这个约定。这是 Tahoe 最容易被忽略、但一眼能看出代次的特征。

```html
<button class="popover__item">
  <span class="popover__icon">…13px 图标…</span>
  <span class="popover__label">显示小组件</span>
  <span class="shortcut">⌘,</span>
</button>
```

```css
.popover__item { display: flex; align-items: center; gap: 8px; }
.popover__icon { flex: 0 0 16px; width: 16px; height: 16px;
                 display: grid; place-items: center; opacity: .8; }
.popover__label { flex: 1 1 auto; }
```

三个要点：

1. **图标列必须定宽**（16px），否则每个菜单项的文字左边缘对不齐，比不加图标更难看。
2. **勾选项的对勾画在同一列**，取代图标 —— macOS 就是这么做的，
   而不是"对勾 + 图标"两个符号并排。
3. 加了图标列之后菜单会变宽，`min-width` 从 200px 提到 **224px**，否则中文标签会折行。

### 其它必需细节

- **当前活跃 App 的名字加粗**（`font-weight: 700`），其余菜单项 500。
  切换窗口时菜单栏内容整体更换 —— 这是 macOS 的灵魂，成本极低但"像"的程度提升明显。
- 左侧：苹果图标 → 活跃 App 名 → 文件 / 编辑 / 显示 / 前往 / 窗口 / 帮助
- 右侧：状态图标（Wi-Fi / 电池 / 控制中心 / 搜索）→ 日期时间
- 菜单项 hover 是 `rgba(255,255,255,.22)` 的圆角块（浅色主题改用 `rgba(0,0,0,.1)`）。
- 状态图标用**单色 SVG**（`fill: currentColor`），不要用 emoji —— 彩色 emoji 会立刻破坏质感。
- 日期时间用 `font-variant-numeric: tabular-nums` 防止数字跳动时宽度抖动。

### 下拉菜单面板

面板级玻璃：`blur(24px) saturate(180%)`，圆角 12px，投影 `0 10px 34px rgba(0,0,0,.34)`。
菜单项 hover 用**实心强调色药丸** `background: var(--mac-blue); color: #fff`。

---

## 7. Dock 规范

macOS 与 iPadOS 形态的 Dock 差别极大，**两种都实测过**，按目标形态取用：

### 7.1 macOS 形态（玻璃底座）

```
容器      高 64px，圆角 20px，底部间距 8px，水平居中，玻璃底座
图标      48pt（Apple 默认值，用户可调 16~128pt），连续曲率 squircle
图标间距  8px
内边距    上下各 8px（48 + 8 + 8 = 64）
```

> 线上站点的 mac26 模式用的是 56px 图标 + 74px 容器。**Apple 的默认值是 48pt**，
> 按 1 CSS px ≈ 1 pt 对齐才不会整体偏大 —— 桌面 UI 里"所有东西都大一号"
> 是最难自查、但旁观者一眼能看出的问题。

### 7.2 iPadOS 形态（无底座，本项目当前采用）

对 `ios.25pan.com` 主屏幕形态的实测（1600×1000 视口）：

```
容器      无玻璃底座、无边框、无圆角 —— 图标直接悬浮在壁纸上
图标      60px，border-radius 25.3%，drop-shadow(rgba(0,0,0,.2) 0 1.8px 3.6px)
图标间距  16px（中心距 76px）
废纸篓    前置 1px 分隔线，尺寸略大（72px），同样无底座
容器高    84px（含运行指示点的空间），底部间距 16px
```

**"去掉底座"是这轮还原里反直觉但最关键的一步**：玻璃底座 + 白描边会让 Dock
看起来像"贴在屏幕上的一块板"，而参考站点的 Dock 是"图标浮在桌面上"。
没有底座时，**每枚图标的 drop-shadow 必须补上**（约 2px 偏移、4px 模糊、20% 黑），
否则图标与亮色壁纸会糊在一起。

### 放大效果（高斯衰减）

macOS 的 Dock 放大是一条平滑的钟形曲线，不是线性缩放。
用高斯函数按"鼠标到图标中心的距离"算缩放比：

```js
const d = Math.abs(cursorX - centerX[i]);
const s = 1 + 0.5 * Math.exp(-Math.pow(d / sigma, 2));   // sigma ≈ 图标边长 × 1.7
icon.style.setProperty("--s", s.toFixed(3));
```

- `0.5` 是最大放大增量（峰值 1.5×），`sigma` 是影响半径。
  **`sigma` 必须跟着图标尺寸走**（本项目 `--dock-icon * 1.7`），
  写死一个常数会在改图标尺寸后手感突变。
- **放大要改宽度而不是 `transform: scale()`**：改宽度会把相邻图标推开（macOS 的真实行为），
  单纯 scale 会让图标互相重叠。代价是重排，但 Dock 只有十来个元素，可以接受。
- 过渡用 `90ms linear`，跟随鼠标不能有延迟感。
- 鼠标离开 Dock 时把所有 `--s` 复位为 1。

> ⚠️ **一个隐蔽的正反馈 bug**：如果按"鼠标到*当前*图标中心"算距离，
> 而图标中心又因为放大而移动，就会形成反馈回路 —— 鼠标划过的过程中图标会抖。
> 正确做法是用**静止槽位**的中心。本项目不解 DOM：
> Dock 靠 `left:50% + translateX(-50%)` 居中，中心恒等于视口中心；
> 槽位等宽，于是静止中心可以直接算出来：
>
> ```js
> const icon = token("--dock-icon", 48);
> const gap  = parseFloat(getComputedStyle(items).columnGap) || 8;
> const cx   = innerWidth / 2;
> const centers = Array.from({ length: n }, (_, i) => cx + (i - (n - 1) / 2) * (icon + gap));
> ```

### 三个必备细节

1. **运行中指示点**：图标下方 4px 圆点，`opacity` 切换，不要用 `display` 切换（会跳）。
2. **通知徽标**：右上角红色药丸 `--mac-red`，`min-width: 18px`，数字居中。
3. **悬浮提示**：延迟出现的独立玻璃气泡，定位在图标上方 30px。

> **放大时容器不跟着长高**：图标用 `align-items: flex-end` 从底部向上生长，
> 超出 Dock 条的部分自然浮在玻璃之上 —— 这与 macOS 一致，
> 也让 Dock 的高度在任何悬停状态下都保持稳定（不会整条跳动）。

---

## 8. 桌面图标与小组件

### 8.1 macOS 形态

- 桌面图标是"图标 + 标签"的纵向组合，标签**必须带 `text-shadow`**
  （`0 1px 3px rgba(0,0,0,.75)`），否则在浅色壁纸上不可读。
- 选中态：图标加 `outline: 2px solid rgba(255,255,255,.5)`，标签变成强调色药丸。
- 桌面图标容器用 `direction: rtl` + `grid-auto-flow: column`，
  可以让"从右向左、每列向下排列"的顺序自然成立，不必手动算格子坐标。
- 小组件是玻璃卡片：圆角 21px、`padding: 14px`、`font-size: 12px`。
  内部文字用 `--text-on-glass`（深色主题下为纯白），因为玻璃底是深色的。

### 8.2 iPadOS 形态：瓷砖网格（本项目当前采用）

对 `ios.25pan.com` 主屏幕形态的实测 —— **桌面不是"图标区 + 小组件区"两块，
而是一张统一的瓷砖网格**，小组件与 App 图标是同一种网格的不同跨度：

```
基础单元    91 × 91px
网格间距    23px（中心距 114px）
网格起点    状态栏下方 31px（y = 59），左缘 14px
小组件卡    2×2 / 2×4 / 3×2 / 4×2 单元拼合，圆角 22.05px
App 图标    1×1 单元：图标 60px 顶部对齐 + 11px 白色标签
玻璃卡配方  rgba(255,255,255,.04) 底 + blur(3px)
            + 0.8px rgba(255,255,255,.48) 描边
            + 5 层 inset 高光 + inset 0 0 2px black/80% + 0 4px 8px black/20%
            + brightness(.9)
```

要点：

1. **用同一张 CSS Grid 承载小组件与图标**（`grid-template-columns: repeat(auto-fill, 91px)`
   + `span 2/3/4`），小组件卡和图标自然对齐同一套网格线 —— 参考站点的
   `desk-tile` / `desk-icon-tile` 正是同一坐标系的两类元素。
2. **App 图标标签 11px**、不加玻璃底，图标自带 `drop-shadow`（与 Dock 同款）。
3. 小组件卡的玻璃配方与"文件夹磁贴"实测值一致（上表），比窗口玻璃更"薄"：
   填充只有 4%，模糊只有 3px。

---

## 9. 动效规范

### 统一缓动

```css
--ease-mac: cubic-bezier(0.32, 0.72, 0, 1);  /* 先快后极缓，macOS 的招牌曲线 */
--ease-out: cubic-bezier(0.22, 1, 0.36, 1);  /* 参考站点全站统一曲线（瓷砖/灵动岛） */
```

> 线上站点在瓷砖、灵动岛等 iPadOS 元素上用的是 `cubic-bezier(0.22, 1, 0.36, 1)`，
> 且**瓷砖与灵动岛的时长不同**：瓷砖 0.26s，灵动岛 0.68s（宽度/高度/圆角一起过渡）。
> 窗口系统仍用 macOS 的 `--ease-mac`。

### 时长

| 动作 | 时长 |
|---|---|
| 窗口开关（genie） | 350ms（线上站点 `--window-genie-duration: 350`） |
| 瓷砖按压/位移 | 260ms `cubic-bezier(.22,1,.36,1)`（透明度 160ms） |
| 灵动岛展开/收起 | 680ms `cubic-bezier(.22,1,.36,1)` |
| 面板 / 弹出层 | 200ms |
| hover 反馈 | 140ms |
| Dock 放大跟随 | 90ms linear |

### 窗口开合

```css
@keyframes genie-in  { from { opacity:0; transform: scale(.92) translateY(26px) } to { opacity:1 } }
@keyframes genie-out { to   { opacity:0; transform: scale(.3)  translateY(42vh) } }
```

最小化时把 `transform-origin` 设到对应 Dock 图标的屏幕坐标，
窗口就会"缩进 Dock"而不是原地淡出：

```js
win.style.transformOrigin = `${dockIconCenterX - winRect.left}px ${dockIconTop - winRect.top}px`;
```

### 拖拽性能

拖动时给窗口加 `.dragging { transition: none; will-change: transform, width, height }`。
**有过渡的拖拽会明显跟手不良** —— 这是最常见的"手感差"来源。

---

## 10. 主题与可访问性

线上站点把可访问性做成了**正交互斥的开关**，值得照搬：

| 模式 | 行为 |
|---|---|
| `no-transparency` | 所有玻璃元素移除 `backdrop-filter`，背景换成不透明色 |
| `blur-disabled` | 把所有模糊滤镜置为 `none` |
| `--wallpaper-*` 系列 | 壁纸独立可调：模糊 / 亮度 / 饱和度 / 压暗 / 缩放 |
| 亮度遮罩、护眼遮罩 | **独立 DOM 层**，与壁纸解耦，可单独叠加 |

- 亮度/护眼必须是**独立叠加层**，不要把滤镜写在壁纸元素上 ——
  否则调节亮度会连带改变壁纸的模糊和缩放基准。
- 主题切换只改 `data-theme` 属性，所有颜色走 CSS 变量，**不写第二套样式表**。
- 字体也是一组 CSS 变量（`--font-family-current`），线上站点提供了
  系统 / 思源黑 / 思源宋 / 霞鹜文楷 / 等宽 / 站酷系列等十余套字体栈，
  通过 `<html>` 上的类名（`font-system`、`font-lxgw`…）切换。

### 必须跟随系统偏好：`prefers-reduced-transparency`

这是**实打实的可用性问题，不只是"不够像"**。macOS 在
「系统设置 → 辅助功能 → 显示器 → 降低透明度」打开时会把所有玻璃换成不透明底色；
网页必须跟随，否则对前视/低视力用户是负担。

```css
@media (prefers-reduced-transparency: reduce) {
  .mac-glass, .mac-glass--surface, .mac-glass--panel {
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
    filter: none;
    background: rgba(28, 28, 32, 0.97);
    border-color: rgba(255, 255, 255, 0.14);
  }
  [data-theme="light"] .mac-glass, /* …同理换成浅色不透明底… */ { }
  /* 依赖预模糊底座做磨砂的层此时已无意义，直接隐藏省一次合成 */
  .wallpaper-mesh { display: none; }
}
```

要点：**手动的"降低透明度"开关不能替代这个媒体查询**。
手动开关是产品功能，媒体查询是用户的系统级无障碍设置 ——
两者都要有，且应共用同一套声明。

### 字体：网页版无法回避的平台差异

`-apple-system` 只在 Apple 平台生效。在 Windows 上会落到 Segoe UI，
与 SF Pro 的字宽、字重、字怀都不同 —— 这是**无法用 CSS 完全消除的差异**。
本项目的取舍：

- 字体栈里依次排入 `SF Pro Text / SF Pro Display / Segoe UI Variable Text / Segoe UI / Inter / Roboto`，
  在 Mac 上就是原生效果，在其它平台取最近的替代。
- **不引入 Web 字体**。项目承诺"零外部资源"，且 Inter 与 SF Pro 仍有可见差异，
  付出 300KB + 一次网络请求换来的提升不成比例。
- 更实际的做法是把**字重层级**做对（标题 600 / 菜单栏 500 / 正文 400），
  层级对了，字体本身的差异就不那么显眼。

> 如果确实需要极致接近，唯一的办法是自托管 SF Pro 的替代品并做 `@font-face`
> 子集化 —— 但那已经超出"纯前端演示"的范围了。

---

## 11. 一眼假：最常见的破绽

按"破坏力"从大到小排。做仿 Mac 界面时先自查这张表，比调任何一个具体数值都有效。

| # | 破绽 | 为什么一眼假 | 修法 |
|---|---|---|---|
| 1 | **用 emoji 当图标** | emoji 是彩色位图、由字体渲染、形状不受控、跨平台不一致；和 macOS 图标"统一几何 + 受控渐变 + 连续曲率轮廓"完全是两套语言 | 全部换内联 SVG（见 3.5 节）。**只要有 1 个 emoji，整体就废了** |
| 2 | **图标圆角用 `border-radius` / `<rect rx>`** | 圆角矩形只有 G1 连续，在四个"肩部"有曲率折点；Apple 是 G2 连续，看不出转折。这是"说不上哪里不对但就是不像"的头号来源 | 用贝塞尔拼出连续曲率圆角（见 3.5）。**只写 22% 的 `border-radius` 是无效的** |
| 3 | **桌面 UI 整体大一号** | 凭感觉放大（Dock 图标 56、红绿灯 16px、菜单栏 28px）会让整个界面"笨"；按 1px ≈ 1pt 对齐才对 | 图标 48 / 红绿灯 12 / 菜单栏 24 / 统一工具栏 52 |
| 4 | **壁纸是 CSS 多色相渐变** | 5~8 个色相的 mesh 渐变会变成"彩虹塑料"；macOS 壁纸是 2~3 个相邻色相 + 流动形态 + 颗粒 + 暗角 | 限制色相数量；加 `feTurbulence` 位移让形态有机化；叠颗粒与暗角 |
| 5 | **红绿灯用半透明白当"灰色"、且没有描边** | Apple 的非焦点态是中性深灰填充 + 略亮灰描边；半透明白在深色窗口上会偏亮偏冷。没有描边的纯色圆片像贴纸 | 焦点 `#ed6a5f/#f6be50/#61c555` + 描边 `#e24b41/#e1a73e/#2dac2f`；非焦点深色外观 `#4b4b4d/#5a5a5c` |
| 6 | **玻璃描边过重** | 小控件的 5 层高光铺到 Dock 这种长条上，会变成一圈白描边，玻璃变塑料 | 大面积用收敛变体：高光减到 2~3 条，靠投影撑层次（见 3 节末） |
| 7 | **菜单栏还是老式毛玻璃横条** | macOS 26 的菜单栏是**完全透明**的，没有 blur；下拉菜单才保留玻璃 | 去掉菜单栏的 `backdrop-filter`，改按壁纸亮度切换字色（见 6 节） |
| 8 | **菜单项没有图标列** | macOS 26 起菜单项左侧统一有 SF Symbol，形成可扫读的一列；这是代次特征 | 加 16px 定宽图标列，勾选对勾占用同一格（见 6 节） |
| 9 | **焦点/非焦点窗口没区别** | macOS 的全部层次感都来自这一处 | 焦点 `0 8px 24px rgba(0,0,0,.42)`，非焦点降到 `0 4px 14px rgba(0,0,0,.26)`，且红绿灯整体变灰 |
| 10 | **菜单栏不跟随活跃应用** | macOS 菜单栏左侧第一项永远是当前 App 名且**加粗**，切换窗口时整排菜单都换 | 切窗口时重建菜单栏，App 名 `font-weight: 700`，其余 500 |
| 11 | **圆角一刀切（到处 8px）** | macOS 是分层圆角，各组件不同 | 窗口 16 / 侧栏 12 / Dock 20 / 小组件 21 / 图标边长 22.37% 连续曲率路径 |
| 12 | **字重单一（全 400）+ 默认字体** | macOS 用字重和字号拉开层级 | 系统字体栈打头；标题 600、菜单栏 500、正文 400；小字用 `letter-spacing: .02em` |
| 13 | **动效线性或 ease-in-out，时长还各不相同** | macOS 是一条"先快后极缓"的曲线，全局复用 | 统一 `cubic-bezier(.32,.72,0,1)`；窗口 350ms / 面板 200ms / hover 140ms |
| 14 | **数字宽度会跳** | 时钟每秒重排，整行左右抖动 | `font-variant-numeric: tabular-nums` |
| 15 | **桌面图标文字在浅色壁纸上读不清** | 没有描边保护 | 标签加 `text-shadow: 0 1px 3px rgba(0,0,0,.75)` |
| 16 | **拖拽带 transition** | 拖窗口"跟不上手"，最典型的"网页感" | 拖动时加 `.dragging { transition: none; will-change: transform }` |
| 17 | **图标用 `box-shadow` 投影** | 图标四角是透明的，`box-shadow` 会画出一个方影子 | 用 `filter: drop-shadow()`，让它跟随形状 |
| 18 | **状态图标用彩色 emoji** | 菜单栏 Wi-Fi / 电池一旦是彩色的，立刻出戏 | 单色内联 SVG + `fill: currentColor` |
| 19 | **侧栏紧贴窗口边缘** | 近两代 macOS 的侧栏是"浮在窗口里的一块玻璃"，比窗口内缩约 8px，自带圆角与投影 | 给侧栏留内边距 + 独立圆角和投影 |
| 20 | **Dock 放大时鼠标划过图标会抖** | 用放大后的位置反推距离会形成正反馈 | 用静止槽位中心算距离（见 7 节） |
| 21 | **整扇窗口都是半透明玻璃** | macOS 只有侧栏/工具栏讲 vibrancy，内容区是不透明的；整窗透明会让窗口和壁纸糊成一片，层次尽失 | 窗体 0.9 不透明，侧栏才是 vibrancy 面板（见 2.2.1） |
| 22 | **折射用 `feTurbulence` 噪声当位移图** | 那是随机抖动，不是光穿过玻璃；真实玻璃表面光滑连续，噪声位移看起来像热浪 | 用形状 SDF 的梯度做透镜剖面（见 3.2） |
| 23 | **四条边等亮的描边** | 光必须来自一个光源；等亮描边会把玻璃变成塑料贴纸 | 只让朝光那一侧亮，用 `max(0, 法线·光源)^n`（见 3.2） |
| 24 | **壁纸是一团柔和渐变（没有结构）** | 折射作用在背景上，背景没结构 = 折射看不见；所有玻璃都会退回磨砂色块 | 壁纸分两层：背景柔光团 + 前景流带（只轻模糊，保住边缘） |
| 25 | **图标各画各的，没有统一受光** | Apple 图标成套靠的是统一材质层，不是每枚画得更细 | 外壳里统一叠一层顶亮底暗的渐变（见 3.5） |

> 这张表里第 1、2、3 条占的比重最大。如果只能改三件事，就改这三件。
>
> 第 21、24 条是**结构性**问题（透明度分配、壁纸结构），单靠调参数补不回来，
> 但它们恰好是「说不上哪里不对」时最常见的原因，优先排查。

---

## 12. 落地检查清单

搭一个新页面时，按顺序过一遍（前四条是决定性的）：

- [ ] **图标全部内联 SVG**（应用 64×64 **连续曲率路径**，界面符号 16×16 线性 `stroke: currentColor`），零 emoji
- [ ] **先定形态再定尺寸**：macOS 形态（Dock 图标 48 / 红绿灯 12·间距 8 / 菜单栏 24 / 工具栏 52）
      或 iPadOS 形态（状态栏 28 / 瓷砖单元 91·间距 23 / Dock 图标 60·间距 16 / 灵动岛 196×34）——
      不要混着用
- [ ] **壁纸只用 2~3 个相邻色相**，叠颗粒与暗角；不用多色相 mesh
- [ ] 壁纸做**两层**：清晰层 + 低分辨率玻璃底座（用 canvas 光栅化一次，底座做到 120×76 级别，靠放大插值代替模糊滤镜）
- [ ] squircle 用 `squirclePath()` 生成贝塞尔路径，`clipPath` 统一在外壳里加；渐变 id 按图标命名空间化
- [ ] 建立 `tokens.css`：强调色、表面色、圆角、阴影、缓动、字体
- [ ] 抽出 `.mac-glass`（小控件：5 层 inset 高光 + specular 径向高光 + `blur(2px)` + `brightness(.9)`）
- [ ] 抽出 `.mac-glass--surface`（大面积：高光收敛 + 投影加重）
- [ ] **窗口透明度按职责分配**：窗体 ~0.9 不透明、标题栏半透明、只有侧栏是 vibrancy 面板
- [ ] 可选（Chromium）：SDF 梯度生成透镜位移贴图 + `feDisplacementMap` + 单光源边缘光，
      用 `@supports (backdrop-filter: url(#x))` 渐进增强，并在开启时让位给清晰壁纸
- [ ] 壁纸必须有**结构**（背景柔光 + 前景流带双层），否则折射与玻璃都看不出来
- [ ] 玻璃面片用独立 `<span>` + `pointer-events: none`，与交互层解耦
- [ ] 菜单栏/状态栏：macOS 24px 或 iPadOS 28px、**透明无 blur**、**按壁纸亮度自适应字色**、13px/500、活跃 App 名加粗并跟随切换
- [ ] iPadOS 形态：灵动岛**纯黑实心、只有下侧两角圆角**，展开改 width/height（不用 scale），三段式顶栏 + 整屏 scrim 对比层
- [ ] 菜单项 / 右键菜单：**16px 定宽前导图标列**，勾选对勾占用同一格
- [ ] 窗口：16px 圆角、统一标题栏、红绿灯直径 12 · 间隙 8（圆心距边 20）、焦点靠投影区分
- [ ] 侧栏：比窗口内缩 8px、自带圆角与投影，是"浮在窗口里的玻璃"
- [ ] Dock：macOS 玻璃底座 64px 或 iPadOS **无底座**（图标 60px 自带 drop-shadow）、高斯衰减放大（改宽度而非 `scale`，用**静止槽位**算距离）、运行指示点、通知徽标
- [ ] 焦点/非焦点两套投影，非焦点窗口红绿灯变灰（真灰色 + 灰描边）
- [ ] 所有动效共用 `cubic-bezier(.32,.72,0,1)`（macOS）或 `cubic-bezier(.22,1,.36,1)`（iPadOS 元素）
- [ ] 拖拽时关闭过渡
- [ ] 提供"降低透明度 / 关闭模糊 / 关闭玻璃快照"手动开关，**并且**实现 `@media (prefers-reduced-transparency: reduce)`
- [ ] 桌面文字加 `text-shadow`，数字用 `tabular-nums`

---

## 13. 数据来源

### 13.1 线上站点实测（第一手）

采集环境：`https://ios.25pan.com/`，1280×720 视口，深色主题，
`data-window-style="mac26"`，`data-dock-style="glass"`，`--blur-enabled: 1`。
数值取自 `getComputedStyle()` 与页面内联 CSS 变量，非推测。

| 元素 | 实测值 |
|---|---|
| 菜单栏 | 高 28px，`z-index: 35`，`font: 13px/500`，`padding: 0 16px` |
| 窗口外壳 | 1120×672 @ (80,24)，圆角 16px，`0 8px 24px rgba(0,0,0,.42)` |
| 侧栏 | 200×656，圆角 12px，`rgba(255,255,255,.19)`，`blur(40px)`，`0 2px 16px rgba(0,0,0,.3)` |
| 红绿灯 | 16px 圆，轴距 24px，`#FF5F57` / `#FFBD2E` / `#28C840` |
| Dock 容器 | 481×74，圆角 20.16px |
| Dock 玻璃面 | `#ffffff0a`，`blur(2px)`，`0.5px rgba(255,255,255,.48)`，`brightness(.9)` |
| 桌面文件夹磁贴 | 164×167，圆角 20.66px，`blur(3px)` |
| 小组件 | 圆角 21.04px |
| 玻璃快照变量 | `--desktop-glass-snapshot`，base64 JPEG 约 160 KB |
| 图标 squircle | `--desk-icon-squircle-percent: 15`，`clip-path: inset(0 round 15%)` |

> 说明：以上仅为**设计参数**的测量与归纳。本项目的 `index.html` 是依据这些参数
> 独立实现的一套原创代码，未复制该站点的任何源码。

### 13.2 与 Apple 实际规格的差异（本项目已按此修正）

该站点的参数**并非全部正确** —— 它是同类 Web 克隆，不是 Apple 官方。
下表列出经开源资料交叉验证后本项目采用的权威值：

| 项目 | 线上站点 | Apple 实际 / 本项目 | 出处 |
|---|---|---|---|
| 图标轮廓 | `clip-path: inset(0 round 15%)`（圆角矩形） | 边长 × 22.37% 的 **G2 连续曲率贝塞尔路径** | figma-squircle；liamrosenfeld.com 逐像素反向工程 |
| 红绿灯直径 | 16px | **12pt** | lwouis/macos-traffic-light-buttons-as-SVG |
| 红绿灯中心距 | 24px | **20pt**（间隙 8pt） | 同上 |
| 红绿灯配色 | `#FF5F57/#FFBD2E/#28C840` | **`#ed6a5f/#f6be50/#61c555`**，描边 `#e24b41/#e1a73e/#2dac2f` | 同上 |
| Dock 图标 | 56px | **48pt**（Apple 默认值） | Apple HIG / macOS 默认值 |
| 菜单栏 | 28px 毛玻璃条 | **24pt，完全透明，字色随壁纸亮度自适应** | WWDC25 session 310 |
| 菜单项 | 无图标 | **左侧 SF Symbol 图标列** | WWDC25 session 310 |
| 无障碍 | 仅有手动开关 | 还需响应 **`prefers-reduced-transparency`** | Apple HIG |

### 13.3 参考的开源项目与资料（附许可）

| 来源 | 用途 | 许可 |
|---|---|---|
| [figma-squircle](https://github.com/MobileReality/figma-squircle) | 连续曲率圆角路径算法，`squirclePath()` 的实现依据 | MIT |
| [s1gmamale1/apple-design-skills](https://github.com/s1gmamale1/apple-design-skills) | macOS 26 / Liquid Glass / SF Symbols / 克制原则 等 Apple 设计规范的整理；本项目据此校正了红绿灯、Dock 尺寸、菜单栏与可访问性 | MIT |
| [lwouis/macos-traffic-light-buttons-as-SVG](https://github.com/lwouis/macos-traffic-light-buttons-as-SVG) | 红绿灯按钮的矢量还原与配色取样 | MIT |
| [liamrosenfeld.com — Reverse-engineering the macOS icon mask](https://liamrosenfeld.com/) | squircle 控制点比例的逐像素反向工程结论（n=4 超椭圆不成立的证据） | 文章 / 引用 |
| WWDC25 session 310「Build a SwiftUI app with the new design」 | macOS 26 菜单栏、工具栏、侧栏玻璃的官方行为说明 | Apple 官方资料 |
| [Liquid Glass in the Browser: Refraction with CSS and SVG](https://kube.io/blog/liquid-glass-css-svg) | 用 Snell 定律推导玻璃曲面、预计算位移矢量场、归一化后写入 SVG 位移贴图、`feImage` 需按元素尺寸适配、`scale` = 最大位移像素数 —— 本项目 3.2 节的实现依据 | 文章 / 引用 |
| [Liquid Glass Refraction — carmenansio.com/lab](https://carmenansio.com/lab/liquid-glass) | 反面案例：说明为什么用 `feTurbulence` 噪声当位移图会失败，以及焦散边缘为何难做 | 文章 / 引用 |

> 本项目为**独立实现**：以上资料用于确定设计参数与几何算法，
> 代码均为自行编写，未直接复制上述项目的源码；
> 引用 MIT 许可项目的算法与结论时已在文件注释与本节中注明出处。

---

## 14. 第二轮实测：主屏幕（iPadOS）形态

第一轮实测针对的是该站点的**窗口/桌面（mac26）形态**（13.1 节）。
本项目早期按"macOS 桌面 + 玻璃 Dock + 左侧小组件"实现，观感与参考站点始终有差距；
第二轮（2026-09，1600×1000 视口）把主屏幕形态完整测了一遍才定位到根因：
**参考站点的主屏幕不是 macOS 桌面，而是 iPadOS 主屏幕** —— 顶栏、图标网格、Dock
三者都要换形态，只调参数补不回来。以下为该轮 `getComputedStyle()` + 元素几何实测值，
本项目已按此重构（`index.html` / `desktop.css` / `tokens.css`）。

### 14.1 顶栏三段式与灵动岛

| 元素 | 实测值 |
|---|---|
| 状态栏容器 | 1600×28，全透明，`font: 13px/500`，`color: rgba(255,255,255,.92)`，`padding: 0 16px` |
| 状态栏左段 | 苹果 logo 按钮 28×28 @ x=16；App 名 SPAN；菜单项按钮，右段从 x=1315 起 |
| 电池 | 按钮 29×28；外框 25×13 @ y=8；充电内芯 21×9（`.is-charging`，绿色） |
| 时钟（右段内） | 152×28：`9月20日周日` + `21:26` 两个 SPAN |
| **灵动岛收起** | **196×34 @ (702,0)**，`background: #000`（纯黑实心），`border-radius: 0 0 22px 22px` |
| 灵动岛展开 | 580×118 @ (702,6)，内容轨道 560×88，多页（概览/天气/闹钟/世界时钟/音乐）+ 页点 |
| 灵动岛过渡 | `width/height/border-radius .68s cubic-bezier(.22,1,.36,1)` |
| 概览页三列 | 待办清单 184 宽 / 中央日期 176 宽（日期+时钟+农历+宜忌）/ 天气 196+347 宽 |
| 整屏 scrim | `radial-gradient(circle at 50% 0%, color(srgb 1 1 1/.12), transparent 42%)` + `color(srgb 0 0 0/.16)` |

### 14.2 瓷砖网格

| 元素 | 实测值 |
|---|---|
| 基础单元 | **91×91px**（`desk-tile` 单格），网格间距 **23px**（列中心距 114px） |
| 网格起点 | `desk-tile-layer` 1573 宽 @ (14,67) —— 状态栏下 39px、左缘 14px |
| 小组件卡跨度 | 音乐 433×208（≈4×2 单元）、天气 205×208（2×2）、日历 205×208、卡片 319×208（3×2）、文件夹 91×208（1×2）、205×91（2×1） |
| 磁贴圆角 | **22.05px**（大卡）/ **14.98px**（91px 内部小面） |
| 磁贴过渡 | `transform .26s cubic-bezier(.22,1,.36,1), opacity .16s` |
| 玻璃卡面 | `rgba(255,255,255,.04)` + `blur(3px)` + `0.8px solid rgba(255,255,255,.48)` + `brightness(.9)` |
| 玻璃卡阴影 | `inset 0 1.25px white/62%`、`inset 2px -2px 1px -1px white/48%`、`inset -2px 2px 1px -1px white/32%`、`inset 6px -6px 1px -6px white/22%`、`inset -6px 6px 1px -6px white/20%`、`inset 0 0 2px black/80%`、`0 4px 8px black/20%` |
| App 图标格 | 60px 图标 + **11px/400 白色标签**（`desk-app-card-tile__label` 22×13） |
| 图标投影 | `drop-shadow(rgba(0,0,0,.2) 0 1.8px 3.6px)`（Dock 与桌面同款） |

### 14.3 Dock（无底座形态）

| 元素 | 实测值 |
|---|---|
| 容器 | 472×84 @ (564,900) —— **无背景、无边框、无圆角**（`desk-root__dock` → `dock-wrapper`） |
| 图标 | **60×60**，`border-radius: 25.3%`，`drop-shadow(rgba(0,0,0,.2) 0 1.8px 3.6px)` |
| 间距 | 16px（图标中心距 76px） |
| 垃圾桶 | 72×72（略大），前置分隔 |
| 运行指示 | 图标下方独立圆点（`.is-dot`） |

### 14.4 结构性结论（为什么第一轮"差一口气"）

1. **形态判断先于一切调参。** 参考站点主屏幕 = iPadOS 主屏幕（状态栏 + 灵动岛 +
   瓷砖网格 + 无底座 Dock），不是 macOS 桌面。拿着 macOS 的尺寸表去"贴近"它，
   每一项都差 10~30%，整体就"说不上哪里不对"。
2. **桌面是一张网格，不是两个区域。** 小组件与 App 图标共用 91px 网格线
   （同一坐标系的不同 `span`），小组件不会"靠边、图标不会靠右"。
3. **Dock 去底座。** 玻璃条 + 白描边在参考站点只属于窗口/文件夹磁贴的玻璃语言；
   Dock 的玻璃语言是"无容器 + 图标自带投影"。
4. **灵动岛是实心的。** 全站都在讲玻璃，唯独它是纯黑 —— "唯一的实心元素"
   恰恰是它一眼可辨的原因。
5. **所有圆角按元素实测**，不要全局一个变量：磁贴 22.05 / 磁贴内面 14.98 /
   图标 25.3% / 灵动岛下角 22 —— 各不相同，是"各元素真实 Apple 规格"的投影。
