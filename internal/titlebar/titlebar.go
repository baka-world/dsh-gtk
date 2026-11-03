// Package titlebar gives the window the chrome of a GTK 4 app.
//
// The shell runs on MyGo, whose Linux backend is GTK 3, so the frame is
// GTK 3's: its theme rounds the top corners at 8px where libadwaita rounds
// all four at 12px, and its header bar carries a gradient, a border and a
// hairline outline that GTK 4 dropped. GtkCSS closes the gap in GTK's own
// CSS, loaded with the user's priority (see internal/gtk), and touches
// nothing else: the title bar, its title and its buttons stay GTK's, which
// is what the official desktop app does on Linux — it keeps the system frame
// there, and only replaces the caption on Windows and macOS.
//
// The title the header bar shows is the page's: MyGo follows the page title,
// so dsh's conversation title appears there as the app changes it.
package titlebar

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Enabled reports whether the window chrome is restyled. DSH_GTK_TITLEBAR=0
// leaves it to the desktop's GTK 3 theme, for a desktop whose theme already
// matches, or for telling the two apart.
func Enabled() bool {
	if runtime.GOOS == "darwin" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSH_GTK_TITLEBAR"))) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

// The fills of the dsh sidebar, which the header bar continues: the frame
// and the page then read as one surface, as the official desktop's caption
// does (it uses the same two tokens, #f9fafb and #1b1b1c).
const (
	lightFill = "#f9fafb"
	lightText = "#1d1d1d"
	darkFill  = "#1b1b1c"
	darkText  = "#f9fafb"
)

// GtkCSS is the window chrome in GTK's CSS, for a light or dark appearance.
// It must be loaded after the toolkit is up; see gtk.Style.
func GtkCSS(dark bool) string {
	fill, text := lightFill, lightText
	if dark {
		fill, text = darkFill, darkText
	}
	return `/* A libadwaita window rounds all four corners at 12px; GTK 3's Adwaita
   rounds the top two at 8px and outlines the window with a hairline GTK 4
   does not draw. GTK shapes the window from this radius. */
decoration {
  border-radius: 12px;
  box-shadow: 0 3px 9px 1px rgba(0, 0, 0, 0.28);
}

/* Maximized, full screen and tiled windows are square, as in libadwaita. */
.maximized decoration, .fullscreen decoration,
.tiled decoration, .tiled-top decoration, .tiled-right decoration,
.tiled-bottom decoration, .tiled-left decoration { border-radius: 0; }

/* Menus, popovers and tooltips keep the radius their theme gives them: the
   rule above would otherwise round every popup like a window. */
.csd.popup decoration, menubar .csd.popup decoration, tooltip.csd decoration { border-radius: 5px; }
.solid-csd decoration { border-radius: 0; }

/* The header bar: libadwaita's flat fill, without GTK 3's gradient, border
   and inner highlight, at libadwaita's 46px. It takes the sidebar's fill so
   that the frame and the page read as one surface. */
headerbar, .titlebar:not(headerbar) {
  min-height: 46px;
  padding: 0 6px;
  background-color: ` + fill + `;
  background-image: none;
  border-width: 0;
  box-shadow: none;
}
headerbar:backdrop, .titlebar:backdrop:not(headerbar) {
  background-color: ` + fill + `;
  background-image: none;
  box-shadow: none;
}
headerbar .title, .titlebar .title { font-weight: bold; color: ` + text + `; }
headerbar .subtitle, .titlebar .subtitle { color: alpha(` + text + `, 0.7); }

/* The window buttons: libadwaita's 24px circles, three pixels apart, with
   no border or gradient, and a neutral hover. */
button.titlebutton {
  min-width: 24px; min-height: 24px;
  margin: 0 3px; padding: 0;
  border: none; border-radius: 9999px;
  background-image: none; background-color: transparent;
  box-shadow: none; text-shadow: none; -gtk-icon-shadow: none;
  color: ` + text + `;
  transition: background-color 120ms ease-out;
}
button.titlebutton:hover { background-color: alpha(` + text + `, 0.1); }
button.titlebutton:active { background-color: alpha(` + text + `, 0.18); }
button.titlebutton:backdrop { opacity: 0.5; }
button.titlebutton:backdrop:hover { background-color: transparent; }

/* GTK 3's menu bar under the title bar belongs to no GTK 4 app: it stays out
   of sight, and Alt or F10 still reveals it. */
menubar { background-color: ` + fill + `; border-width: 0; box-shadow: none; }
`
}

// Script is injected into every page before the page's own scripts: it
// rounds the page's bottom corners, which GTK 3 cannot do itself (it draws
// the content area as an opaque rectangle and never clips a child to the
// decoration's radius, so only the top two corners round).
//
// The window is transparent under the page (see WindowOptions.Transparent),
// so the rounded corners show the desktop; GTK still draws the title bar,
// the window buttons and the shadow around the page.
var Script = `(() => {
  'use strict';
  if (window.__dgoCorners || window.top !== window) return;
  window.__dgoCorners = true;
  const css = ` + cornerCSSLiteral + `;
  const install = () => {
    if (document.getElementById('dgo-corners')) return true;
    const style = document.createElement('style');
    style.id = 'dgo-corners';
    style.textContent = css;
    (document.head || document.documentElement).appendChild(style);
    return true;
  };
  if (!install()) {
    const observer = new MutationObserver(() => { if (install()) observer.disconnect(); });
    observer.observe(document, { childList: true, subtree: true });
  }
  // A maximized or full-screen window sits against the screen edge, where a
  // rounded corner would leave the desktop showing: the window says so.
  addEventListener('dgo:rounded', (event) => {
    document.documentElement.toggleAttribute('data-dgo-square', !event.detail);
  });
})();`

// cornerCSS rounds the bottom corners the way libadwaita rounds a window.
// The radius is on html, body and the app's root, and overflow clips what
// they paint: dsh's shell paints on the frame inside #root.
const cornerCSS = `
html { background: transparent !important; }
html, body, #root {
  border-bottom-left-radius: 12px;
  border-bottom-right-radius: 12px;
}
html, body { overflow: hidden; }
html[data-dgo-square], html[data-dgo-square] body, html[data-dgo-square] #root {
  border-bottom-left-radius: 0;
  border-bottom-right-radius: 0;
}
`

// RoundedScript tells the page of a window that maximized or left full
// screen, where its bottom corners must be square.
func RoundedScript(rounded bool) string {
	return fmt.Sprintf(
		`window.dispatchEvent(new CustomEvent('dgo:rounded',{detail:%t}))`,
		rounded,
	)
}

var cornerCSSLiteral = strconv.Quote(cornerCSS)
