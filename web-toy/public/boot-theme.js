// BeaconTower · toy 主题同步 boot（doc/18）
//
// 必须放在 index.html 里「模块加载前」同步执行，才能防住错误主题闪屏。
// 之所以是同源外部脚本而非内联：应用自身的 CSP 是 default-src 'self'
// （无 script-src / 无 nonce），内联脚本会被拦截而静默失效（v2.8 实为如此）。
// 由后端按主题作用域提供（/toy/boot-theme.js）。
//
// 1) ?theme=<id> → 写偏好并 replace 到该主题路径（清参防重复触发）
// 2) 无参且偏好 ≠ 当前主题 → replace 过去。仅在「偏好 ≠ 当前」时跳，
//    双向只会发生一次跳转，无重定向死循环。
(function () {
  try {
    // dev 双端口（classic 5173 / toy 5174）各服务一套前端：跨主题切换由两侧
    // 切换入口按端口直跳，boot 不做路径迁移，否则会对同源被重定向的路径刷屏。
    if (window.location.port === '5173' || window.location.port === '5174') return;
    var THEMES = {
      toy: { path: '/toy/', current: true },
      classic: { path: '/', current: false },
    };
    var qs = new URLSearchParams(window.location.search);
    var wanted = qs.get('theme');
    if (wanted && THEMES[wanted]) {
      localStorage.setItem('beacontower_preferred_theme', wanted);
      qs.delete('theme');
      var rest = qs.toString();
      var target = THEMES[wanted].path + (rest ? '?' + rest : '');
      if (target !== window.location.pathname + window.location.search) {
        window.location.replace(target);
      }
      return;
    }
    var pref = localStorage.getItem('beacontower_preferred_theme');
    if (pref && THEMES[pref] && !THEMES[pref].current) {
      window.location.replace(THEMES[pref].path);
    }
  } catch (e) {
    /* localStorage 不可用（隐私模式）：原地渲染当前主题 */
  }
})();
