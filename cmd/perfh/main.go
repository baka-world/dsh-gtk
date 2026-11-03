// Command perfh is a throwaway harness: it loads the dsh workspace, scrolls
// it, and reports frame-time percentiles plus DOM facts, optionally with
// injected CSS.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/baka-world/dsh-gtk/internal/dsh"
	"github.com/baka-world/dsh-gtk/internal/warmup"
	"github.com/egoist/mygo"
)

type stats struct {
	Frames   int     `json:"frames"`
	Ms       float64 `json:"ms"`
	FPS      float64 `json:"fps"`
	P50      float64 `json:"p50"`
	P95      float64 `json:"p95"`
	P99      float64 `json:"p99"`
	Low1     float64 `json:"low1"`
	Max      float64 `json:"max"`
	Long50   int     `json:"long50"`
	Long100  int     `json:"long100"`
	Nodes    int     `json:"nodes"`
	Pre      int     `json:"pre,omitempty"`
	Shiki    int     `json:"shiki,omitempty"`
	Scroller bool    `json:"scroller"`
	Moved    int     `json:"moved"`
	Hitches  []hitch `json:"hitches,omitempty"`
}

type hitch struct {
	Gap    float64 `json:"g"`
	Top    int     `json:"top"`
	El     string  `json:"el"`
	Muts   int     `json:"muts,omitempty"`
	MutAge float64 `json:"mutAge,omitempty"`
	MutEl  string  `json:"mutEl,omitempty"`
	Sh     int     `json:"sh,omitempty"`
}

const script = `(function(){
  var t0 = performance.now(), started = 0, gaps = [], last = 0, scroller = null, dir = 1, moved = 0, hitches = [];
  var muts = 0, lastMut = 0, mutEl = '';
  try {
    new MutationObserver(function(list) {
      muts += list.length; lastMut = performance.now();
      var n = list[list.length - 1], t = n.target, a = n.addedNodes[0];
      mutEl = (t.tagName || '') + '.' + String(t.className || '').slice(0, 40) + ' +' + n.addedNodes.length +
        (a ? ' -> ' + (a.tagName || a.nodeName || '') + '.' + String(a.className || '').slice(0, 40) : '');
    }).observe(document.documentElement, {childList: true, subtree: true, characterData: true});
  } catch (e) {}
  function diag() {
    var all = document.querySelectorAll('*'), se = document.scrollingElement || document.documentElement;
    var best = null, bestScore = 0;
    for (var i = 0; i < all.length; i++) {
      var el = all[i], cs = getComputedStyle(el), r = el.getBoundingClientRect();
      var d = el.scrollHeight - el.clientHeight;
      if (d < 30) continue;
      var score = d * Math.max(0, r.width) * Math.max(0, r.height) * ((cs.overflowY === 'auto' || cs.overflowY === 'scroll') ? 4 : 1);
      if (score > bestScore) { best = el; bestScore = score; }
    }
    var out = {nodes: all.length, se: [se.scrollHeight, se.clientHeight]};
    if (best) {
      out.sb = {tag: best.tagName, cls: String(best.className||'').slice(0,100), sh: best.scrollHeight, ch: best.clientHeight, kids: best.childCount, attrs: best.getAttributeNames().join(','), conv: best.hasAttribute('data-conversation-scroll'), q: !!document.querySelector('[data-conversation-scroll]')};
      var chain = [], e = best;
      for (var i = 0; i < 4 && e; i++, e = e.parentElement) chain.push(e.tagName + '.' + String(e.className||'').slice(0,60));
      out.chain = chain;
      var c = best.firstElementChild;
      if (c) out.firstKid = {tag: c.tagName, cls: String(c.className||'').slice(0,80), kids: c.childCount,
        kid0: c.firstElementChild ? c.firstElementChild.tagName + '.' + String(c.firstElementChild.className||'').slice(0,80) : ''};
      var grand = c && c.firstElementChild;
      if (grand) out.grandKid = {tag: grand.tagName, cls: String(grand.className||'').slice(0,80), kids: grand.childCount};
    }
    var fx = {shadow:0, backdrop:0, filter:0, anim:0, img:0, sticky:0, will:0};
    for (var j = 0; j < all.length; j++) {
      var cs = getComputedStyle(all[j]);
      if (cs.boxShadow && cs.boxShadow !== 'none') fx.shadow++;
      if (cs.backdropFilter && cs.backdropFilter !== 'none' && cs.backdropFilter !== '') fx.backdrop++;
      if (cs.filter && cs.filter !== 'none') fx.filter++;
      if (cs.animationName && cs.animationName !== 'none') fx.anim++;
      if (all[j].tagName === 'IMG' || all[j].tagName === 'VIDEO') fx.img++;
      if (cs.position === 'sticky') fx.sticky++;
      if (cs.willChange && cs.willChange !== 'auto') fx.will++;
    }
    out.fx = fx;
    out.pre = document.querySelectorAll('pre').length;
    out.shiki = document.querySelectorAll('pre.shiki').length;
    var cc = {};
    for (var k = 0; k < all.length; k++) {
      String(all[k].className||'').split(/\s+/).forEach(function(x){ if (x) cc[x] = (cc[x]||0)+1; });
    }
    out.top = Object.keys(cc).map(function(k){ return [k, cc[k]]; }).sort(function(a,b){ return b[1]-a[1]; }).slice(0, 12);
    return out;
  }
  function pick() {
    var all = document.querySelectorAll('*'), best = null, bestScore = 0;
    for (var i = 0; i < all.length; i++) {
      var el = all[i], cs = getComputedStyle(el), r = el.getBoundingClientRect();
      var d = el.scrollHeight - el.clientHeight;
      if (d < 30) continue;
      var score = d * Math.max(0, r.width) * Math.max(0, r.height) * ((cs.overflowY === 'auto' || cs.overflowY === 'scroll') ? 4 : 1);
      if (score > bestScore) { best = el; bestScore = score; }
    }
    return best;
  }
  function pct(a, q) { return a[Math.min(a.length - 1, Math.max(0, Math.floor(q * a.length)))]; }
  function tick(now) {
    if (!started && window.__dshGoWarmup && window.__dshGoWarmup !== 'done') {
      if (performance.now() - t0 < 20000) { setTimeout(function() { requestAnimationFrame(tick); }, 100); return; }
    }
    if (!started) {
      scroller = pick();
      if (!scroller && now - t0 < 8000) { requestAnimationFrame(tick); return; }
      new Image().src = 'http://127.0.0.1:8977/diag?d=' + encodeURIComponent(JSON.stringify(diag()));
      if (scroller && __WARM__) {
        var mode = (__WARM__ === true || __WARM__ === 'true') ? 'visible' : __WARM__;
        var top0 = scroller.scrollTop, vis0 = scroller.style.visibility, op0 = scroller.style.opacity;
        var ov = null;
        if (mode === 'overlay') {
          ov = document.createElement('div');
          ov.id = 'perfh-warm';
          ov.textContent = 'Warming the conversation…';
          ov.style.cssText = 'position:fixed;inset:0;z-index:2147483647;background:#0f1117;color:#e6e8ee;' +
            'display:flex;align-items:center;justify-content:center;font:15px system-ui';
          document.body.appendChild(ov);
        } else if (mode === 'cover') {
          var r = scroller.getBoundingClientRect();
          ov = document.createElement('div');
          ov.style.cssText = 'position:fixed;z-index:2147483647;background:' + getComputedStyle(scroller).backgroundColor +
            ';left:' + r.left + 'px;top:' + r.top + 'px;width:' + r.width + 'px;height:' + r.height + 'px';
          document.body.appendChild(ov);
        } else if (mode === 'hidden') {
          scroller.style.visibility = 'hidden';
        } else if (mode.indexOf('opacity') === 0) {
          scroller.style.opacity = mode === 'opacity001' ? '0.001' : '0';
        }
        scroller.scrollTop = scroller.scrollHeight;
        var restore = function() {
          scroller.scrollTop = top0;
          scroller.style.visibility = vis0;
          scroller.style.opacity = op0;
          if (ov) ov.remove();
          started = performance.now(); last = started; requestAnimationFrame(tick);
        };
        if (mode === 'visible' || mode === 'opacity' || mode === 'opacity001') {
          requestAnimationFrame(function() { requestAnimationFrame(restore); });
        } else if (mode === 'opacity100') {
          setTimeout(restore, 100);
        } else {
          setTimeout(restore, 700);
        }
        return;
      }
      started = now; last = now;
    }
    if (last) {
      var g = now - last;
      gaps.push(g);
      if (g > 40) {
        var el = document.elementFromPoint(window.innerWidth / 2, window.innerHeight / 2);
        hitches.push({g: Math.round(g), top: scroller ? Math.round(scroller.scrollTop) : 0,
          el: el ? el.tagName + '.' + String(el.className || '').slice(0, 60) : '',
          muts: muts, mutAge: lastMut ? Math.round(now - lastMut) : -1, mutEl: mutEl,
          sh: scroller ? scroller.scrollHeight : 0});
      }
    }
    last = now;
    if (!scroller || !scroller.isConnected) scroller = pick();
    if (scroller) {
      var before = scroller.scrollTop;
      scroller.scrollTop = before + 20 * dir;
      if (scroller.scrollTop !== before) moved++;
      if (scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 2) dir = -1;
      else if (scroller.scrollTop <= 2) dir = 1;
    }
    if (now - started < 12000) { requestAnimationFrame(tick); return; }
    var sorted = gaps.slice().sort(function(a,b){ return a-b; });
    var n1 = Math.max(1, Math.floor(sorted.length * 0.01)), sum1 = 0;
    for (var i = sorted.length - n1; i < sorted.length; i++) sum1 += sorted[i];
    var long50 = 0, long100 = 0;
    for (var j = 0; j < gaps.length; j++) { if (gaps[j] > 50) long50++; if (gaps[j] > 100) long100++; }
    var s = {frames: gaps.length, ms: now - started, fps: gaps.length / ((now - started) / 1000),
      p50: pct(sorted, 0.50), p95: pct(sorted, 0.95), p99: pct(sorted, 0.99), low1: sum1 / n1,
      max: sorted[sorted.length - 1] || 0, long50: long50, long100: long100,
      nodes: document.querySelectorAll('*').length, scroller: !!scroller, moved: moved,
      pre: document.querySelectorAll('pre').length, shiki: document.querySelectorAll('pre.shiki').length,
      hitches: hitches.slice(0, 8)};
    new Image().src = 'http://127.0.0.1:8977/stats?d=' + encodeURIComponent(JSON.stringify(s));
  }
  requestAnimationFrame(tick);
})();`

func main() {
	got := make(chan stats, 1)
	diagc := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/diag", func(w http.ResponseWriter, r *http.Request) {
		select {
		case diagc <- r.URL.Query().Get("d"):
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		var s stats
		if err := json.Unmarshal([]byte(r.URL.Query().Get("d")), &s); err == nil {
			select {
			case got <- s:
			default:
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:8977")
	if err != nil {
		fmt.Println("listen:", err)
		os.Exit(1)
	}
	go http.Serve(ln, mux)

	res, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		fmt.Println("resources:", err)
		os.Exit(1)
	}
	launch, err := dsh.Discover(dsh.DiscoverOptions{Resources: res})
	if err != nil {
		fmt.Println("discover:", err)
		os.Exit(1)
	}
	ws := os.Getenv("PERFH_WS")
	if ws == "" {
		ws = "/tmp/dshws"
	}
	srv, err := dsh.Start(dsh.Config{Launch: launch, Workspace: ws, Port: 4018, Log: os.Stderr})
	if err != nil {
		fmt.Println("start:", err)
		os.Exit(1)
	}
	defer srv.Stop(5 * time.Second)

	css := os.Getenv("PERFH_CSS")
	if p := os.Getenv("PERFH_CSS_FILE"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			css = string(b)
		}
	}
	warm := "false"
	if v := os.Getenv("PERFH_WARM"); v != "" {
		warm = v
	}
	warmJS := warm
	if warm == "product" {
		warmJS = "false"
	} else if warm != "false" && warm != "true" {
		warmJS = fmt.Sprintf("%q", warm)
	}
	pre := strings.ReplaceAll(script, "__WARM__", warmJS)
	if warm == "product" {
		pre = warmup.Script + pre
	}
	if s := os.Getenv("PERFH_SESSION"); s != "" {
		key, _ := json.Marshal("dsh.sessions.current")
		val, _ := json.Marshal(`{"sessionId":"` + s + `"}`)
		pre = "(function(){try{localStorage.setItem(" + string(key) + "," + string(val) + ")}catch(e){}})();" + pre
	}
	if css != "" {
		j, _ := json.Marshal(css)
		pre = "(function(){var s=document.createElement('style');s.textContent=" + string(j) +
			";(document.head||document.documentElement).appendChild(s);})();" + pre
	}

	mygo.App.SetName("DSH GTK")
	mygo.App.WhenReady(func() {
		cookies, err := srv.AuthCookies(10 * time.Second)
		if err != nil {
			fmt.Println("cookies:", err)
			mygo.App.Quit()
			return
		}
		origin := srv.Origin()
		host := "127.0.0.1"
		if u, err := url.Parse(origin); err == nil {
			host = u.Hostname()
		}
		for _, c := range cookies {
			_ = mygo.Cookies.Set(mygo.Cookie{Name: c.Name, Value: c.Value, Domain: host, Path: "/",
				Expires: c.Expires, HTTPOnly: true, SameSite: mygo.CookieSameSiteLax})
		}
		mygo.NewWindow(mygo.WindowOptions{
			Width: 1600, Height: 1000,
			URL:  origin,
			Page: mygo.PageOptions{PreloadScript: pre},
		})
	})

	go func() {
		select {
		case d := <-diagc:
			fmt.Println("DIAG", d)
		case <-time.After(25 * time.Second):
		}
		select {
		case s := <-got:
			b, _ := json.Marshal(s)
			fmt.Println("STATS", string(b))
		case <-time.After(60 * time.Second):
			fmt.Println("STATS timeout")
		}
		mygo.App.Quit()
	}()

	if err := mygo.App.Run(); err != nil {
		fmt.Println("run:", err)
	}
}
