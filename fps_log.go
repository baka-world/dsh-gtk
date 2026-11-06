package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"
)

// fpsReportJS reads the overlay's numbers. It returns "" until the overlay has
// installed itself, so the log stays quiet on the splash and error pages.
const fpsReportJS = `(function () {
  var overlay = window.__dshFPS;
  if (!overlay) return "";
  var r = overlay.report();
  var slow = r.slow.map(function (f) { return Math.round(f.d) + 'ms'; }).join(',');
  return 'fps=' + r.fps + ' nodes=' + r.nodes + ' mutPeak=' + r.mutPeak + ' slow=' + (slow || '-');
})()`

// logFPS writes the overlay's numbers to the app's log once a second, so a
// measurement is a file to read rather than a screenshot (DSH_GTK_FPS_LOG=1).
func (l *Launcher) logFPS(w *mygo.Window) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-t.C:
		}
		v, err := w.Page().Eval(fpsReportJS)
		if err != nil {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			log.Printf("dsh-gtk: fps %s", s)
		}
	}
}
