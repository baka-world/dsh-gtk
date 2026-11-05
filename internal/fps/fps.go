// Package fps is the frame-time overlay DSH_GTK_FPS=1 puts on the dsh
// workspace.
//
// The shell cannot read the page's frame rate from outside: a production
// build carries no WebKit inspector (cmd/mygo builds with mygo_noinspector),
// and the compositor counts presents, not the frames the page produced. The
// overlay measures in the page instead: requestAnimationFrame intervals, the
// frames over one 60 Hz budget, PerformanceObserver long tasks, and the DOM
// size that makes the work. It is the measurement behind docs/perf-sidebar.md
// and the number to quote when a sidebar animation is reported as slow.
package fps

import (
	"os"
	"strings"
)

// Enabled reports whether the overlay is injected. DSH_GTK_FPS=1 (or on/yes/
// true) turns it on; anything else, including unset, leaves it off.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSH_GTK_FPS"))) {
	case "", "0", "off", "false", "no":
		return false
	}
	return true
}

// Script draws the overlay in the page. It runs in every page before the
// page's own scripts and does nothing on mygo:// documents (the splash and
// error pages) or in subframes.
//
// It keeps the last second of frame intervals, so the fps is what the user
// just saw, and a 24 second history of it, which makes a sidebar toggle show
// up as the dip it is. Alt+Shift+F hides it for a screenshot.
const Script = `(function () {
  if (window.top !== window) return;
  if (location.protocol !== 'http:' && location.protocol !== 'https:') return;
  if (window.__dshFPS) return;

  var frames = [];    // {t, d} of the last second
  var history = [];   // fps per second, last 24
  var longs = [];     // long tasks of the last 10s
  var last = 0, worst = 0, worstAt = 0;
  var hud = null, hidden = false, lastHistory = 0;
  var nodes = 0, nodesAt = 0;

  function ensure() {
    if (hud && hud.isConnected) return hud;
    if (!document.body) return null;
    hud = document.createElement('div');
    hud.setAttribute('aria-hidden', 'true');
    hud.style.cssText = [
      'position:fixed', 'top:8px', 'right:8px', 'z-index:2147483647',
      'pointer-events:none', 'user-select:none',
      'font:11px/1.45 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace',
      'color:#fff', 'background:rgba(0,0,0,.66)',
      'padding:5px 7px', 'border-radius:6px', 'white-space:pre',
      'text-shadow:0 1px 2px rgba(0,0,0,.55)',
      'display:' + (hidden ? 'none' : 'block')
    ].join(';');
    document.body.appendChild(hud);
    return hud;
  }

  function tick(now) {
    if (last) {
      var d = now - last;
      frames.push({ t: now, d: d });
      if (d > worst) { worst = d; worstAt = now; }
    }
    last = now;
    window.requestAnimationFrame(tick);
  }

  try {
    new PerformanceObserver(function (list) {
      list.getEntries().forEach(function (e) {
        longs.push({ t: performance.now(), d: e.duration });
        if (e.duration > worst) { worst = e.duration; worstAt = performance.now(); }
      });
    }).observe({ entryTypes: ['longtask'] });
  } catch (e) { /* this engine has no longtask entries */ }

  function quantile(values, q) {
    if (!values.length) return 0;
    var s = values.slice().sort(function (a, b) { return a - b; });
    return s[Math.min(s.length - 1, Math.floor(s.length * q))];
  }

  function mb(bytes) {
    return (bytes / (1024 * 1024)).toFixed(0) + 'M';
  }

  function sample() {
    var now = performance.now();
    frames = frames.filter(function (f) { return f.t >= now - 1000; });
    longs = longs.filter(function (l) { return l.t >= now - 10000; });
    var deltas = frames.map(function (f) { return f.d; });
    var fps = frames.length;
    var jank = deltas.filter(function (d) { return d > 20; }).length;
    var worstNow = deltas.length ? Math.max.apply(null, deltas) : 0;
    var recent = longs.filter(function (l) { return l.t >= now - 5000; });
    var longMs = recent.reduce(function (sum, l) { return sum + l.d; }, 0);
    if (now - lastHistory >= 1000) {
      history.push(fps);
      if (history.length > 24) history.shift();
      lastHistory = now;
    }
    if (now - nodesAt >= 1000) {
      nodes = document.getElementsByTagName('*').length;
      nodesAt = now;
    }
    var el = ensure();
    if (!el) return;
    var bars = history.map(function (v) {
      var i = Math.max(0, Math.min(7, Math.round(v / 60 * 7)));
      return '\u2581\u2582\u2583\u2584\u2585\u2586\u2587\u2588'.charAt(i);
    }).join('');
    var lines = [
      'fps ' + fps + '  p50 ' + quantile(deltas, .5).toFixed(1) + '  p95 ' + quantile(deltas, .95).toFixed(1) + 'ms',
      '>20ms ' + jank + '  max ' + worstNow.toFixed(0) + 'ms  lt ' + recent.length + '/' + Math.round(longMs) + 'ms',
      'dom ' + nodes + (window.performance.memory ? '  heap ' + mb(window.performance.memory.usedJSHeapSize) : '')
    ];
    if (worst - worstNow > 40) {
      lines.push('peak ' + worst.toFixed(0) + 'ms ' + ((now - worstAt) / 1000).toFixed(1) + 's ago');
    }
    if (bars) lines.push(bars);
    el.textContent = lines.join('\n');
  }

  window.__dshFPS = {
    report: function () {
      return { fps: frames.length, history: history.slice(), worst: worst, nodes: nodes };
    },
    hide: function () { hidden = true; if (hud) hud.style.display = 'none'; },
    show: function () { hidden = false; if (hud) hud.style.display = 'block'; }
  };

  document.addEventListener('keydown', function (e) {
    if (e.altKey && e.shiftKey && (e.key === 'F' || e.key === 'f')) {
      window.__dshFPS[hidden ? 'show' : 'hide']();
    }
  }, true);

  window.requestAnimationFrame(tick);
  window.setInterval(sample, 250);
})();
`
