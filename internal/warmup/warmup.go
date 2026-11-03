// Package warmup provides the script a DSH Go window injects into the dsh
// workspace before its own scripts.
//
// dsh renders the end of a long transcript lazily: the first time the last
// turn comes into view, WebKit pays for the layout and paint of that turn in
// one frame. Measured on a 4,586-node conversation, that frame took 213 ms on
// the DMA-BUF renderer and 0.9 s with software rendering, and it showed up as
// a single long frame at the bottom of the transcript.
//
// The script below forces that work while the window is still loading, when
// it is not in the middle of a scroll: it scrolls the transcript to its end
// for one painted frame and puts the reader back. When dsh has already
// scrolled to the end itself, it nudges the scrollport instead, so the
// observers that dsh installed while the transcript was growing run before
// the reader does.
package warmup

import (
	"os"
	"strings"
)

// Script reads the conversation scrollport (dsh marks it with
// data-conversation-scroll), makes it paint its end once, and restores the
// reader's position. It runs at most once per page load, on http pages only
// (the splash and error pages are mygo:// documents), and waits for the
// transcript to render.
//
// The animation frames matter: WebKit lays out and paints the end of the
// transcript only while it is visible, so a hidden or opacity-0 jump warms
// nothing (measured: 55 ms and 97 ms hitches remained, against 26 ms for the
// visible jump).
const Script = `(function () {
  if (location.protocol !== 'http:') return;
  if (window.__dshGoWarmup) return;
  window.__dshGoWarmup = 'pending';

  var started = performance.now();

  function finish() { window.__dshGoWarmup = 'done'; }

  function run() {
    var sc = document.querySelector('[data-conversation-scroll]');
    if (!sc) {
      if (performance.now() - started < 10000) { setTimeout(run, 100); return; }
      finish();
      return;
    }
    // The scrollport exists before the transcript fills it.
    if (sc.scrollHeight - sc.clientHeight < 240) {
      if (performance.now() - started < 10000) { setTimeout(run, 100); return; }
      finish();
      return;
    }
    var top = sc.scrollTop;
    // dsh opens a conversation at its end; WebKit may not have re-evaluated
    // the observers of the blocks that grew into view without a scroll.
    // Moving the scrollport by a pixel makes it, and the reader never sees
    // the position change.
    if (top + sc.clientHeight >= sc.scrollHeight - 2) {
      sc.scrollTop = top - 2;
      requestAnimationFrame(function () {
        requestAnimationFrame(function () {
          sc.scrollTop = top;
          requestAnimationFrame(function () { finish(); });
        });
      });
      return;
    }
    // Opened away from the end: visit it for one painted frame so the last
    // turn is laid out and painted before the reader scrolls there.
    sc.scrollTop = sc.scrollHeight;
    requestAnimationFrame(function () {
      requestAnimationFrame(function () {
        sc.scrollTop = top;
        requestAnimationFrame(function () {
          // dsh may resume following the tail after the jump; hold the
          // reader's place anyway.
          if (Math.abs(sc.scrollTop - top) > 1) sc.scrollTop = top;
          finish();
        });
      });
    });
  }

  // The conversation is rendered after its first data fetch.
  setTimeout(run, 0);
})();`

// Enabled reports whether the warm-up is on. It is on by default;
// DSH_GO_WARMUP=0 (or off/false/no) turns it off for a window whose
// injected scroll is not wanted.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSH_GO_WARMUP"))) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}
