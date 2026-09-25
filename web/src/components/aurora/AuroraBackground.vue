<!-- ============================================================
     天空信标 · 极光背景层（纯装饰，aria-hidden）
     - 4 团马卡龙色光斑（天蓝/薰衣草/薄荷/蜜桃）以不同周期缓慢漂移
     - 顶部一道“信标光束”呼吸，呼应品牌
     - 细点阵纹理只在视口上 2/3 显现，向下渐隐
     性能：全部为 transform/opacity 合成动画，无布局与绘制开销；
     prefers-reduced-motion 下全部静止。
     ============================================================ -->
<template>
  <div class="aurora" aria-hidden="true">
    <div class="aurora__base" />
    <div class="aurora__blob aurora__blob--sky" />
    <div class="aurora__blob aurora__blob--violet" />
    <div class="aurora__blob aurora__blob--mint" />
    <div class="aurora__blob aurora__blob--peach" />
    <div class="aurora__beam" />
    <div class="aurora__dots" />
  </div>
</template>

<style scoped>
.aurora {
  position: fixed;
  inset: 0;
  z-index: 0;
  overflow: hidden;
  pointer-events: none;
}

/* 底色：清晨天空的三段渐变 */
.aurora__base {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(158deg, #e4effd 0%, #eef0fc 38%, #f3eefb 62%, #e9f6f0 100%);
}

.aurora__blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(72px);
  will-change: transform;
}

.aurora__blob--sky {
  width: 52vw;
  height: 52vw;
  min-width: 480px;
  min-height: 480px;
  top: -16%;
  left: -10%;
  background: radial-gradient(circle, rgba(125, 211, 252, 0.55), transparent 68%);
  animation: aurora-drift-a 44s ease-in-out infinite;
}

.aurora__blob--violet {
  width: 44vw;
  height: 44vw;
  min-width: 420px;
  min-height: 420px;
  top: 6%;
  right: -12%;
  background: radial-gradient(circle, rgba(196, 181, 253, 0.5), transparent 68%);
  animation: aurora-drift-b 56s ease-in-out infinite;
}

.aurora__blob--mint {
  width: 46vw;
  height: 46vw;
  min-width: 400px;
  min-height: 400px;
  bottom: -18%;
  left: 12%;
  background: radial-gradient(circle, rgba(110, 231, 183, 0.42), transparent 68%);
  animation: aurora-drift-c 62s ease-in-out infinite;
}

.aurora__blob--peach {
  width: 34vw;
  height: 34vw;
  min-width: 320px;
  min-height: 320px;
  bottom: 4%;
  right: 10%;
  background: radial-gradient(circle, rgba(253, 224, 171, 0.5), transparent 68%);
  animation: aurora-drift-a 50s ease-in-out infinite reverse;
}

/* 信标光束：顶部中央的柔光，缓慢呼吸 */
.aurora__beam {
  position: absolute;
  inset: 0;
  background: radial-gradient(
    ellipse 55% 42% at 50% -10%,
    rgba(56, 189, 248, 0.22),
    rgba(139, 92, 246, 0.08) 55%,
    transparent 75%
  );
  animation: aurora-beam 7s ease-in-out infinite;
}

/* 细点阵：只在上部显现，向下渐隐，增加“塔”的结构感 */
.aurora__dots {
  position: absolute;
  inset: 0;
  background-image: radial-gradient(circle, rgba(51, 65, 110, 0.09) 1px, transparent 1px);
  background-size: 26px 26px;
  mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.7), transparent 68%);
  -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.7), transparent 68%);
}

@keyframes aurora-drift-a {
  0%,
  100% {
    transform: translate(0, 0) scale(1);
  }

  33% {
    transform: translate(5vw, 3vh) scale(1.07);
  }

  66% {
    transform: translate(-3vw, 5vh) scale(0.96);
  }
}

@keyframes aurora-drift-b {
  0%,
  100% {
    transform: translate(0, 0) scale(1);
  }

  50% {
    transform: translate(-6vw, 6vh) scale(1.09);
  }
}

@keyframes aurora-drift-c {
  0%,
  100% {
    transform: translate(0, 0) scale(1);
  }

  40% {
    transform: translate(4vw, -5vh) scale(1.06);
  }

  75% {
    transform: translate(-4vw, -2vh) scale(0.95);
  }
}

@keyframes aurora-beam {
  0%,
  100% {
    opacity: 0.75;
  }

  50% {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .aurora__blob,
  .aurora__beam {
    animation: none;
  }
}
</style>
