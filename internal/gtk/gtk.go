// Package gtk gives the shell the few GTK calls MyGo does not expose.
//
// MyGo's Linux backend is GTK 3, and its public API stops at the window: an
// app cannot restyle the widgets GTK draws around a page. That leaves two
// visible gaps against the GTK 4 apps of a GNOME desktop, both of them in
// the window frame: GTK 3's theme rounds only the top corners of a window
// (8px where libadwaita rounds all four at 12px), and its title buttons are
// laid out by GTK 3's theme.
//
// A style provider loaded here with the user's priority closes them: the
// CSS wins over the theme and over MyGo's own provider (application
// priority), and GTK keeps drawing the widgets, so hover, click, keyboard
// focus, HiDPI and the window menu stay native.
package gtk

import "errors"

// ErrUnsupported is returned where the shell does not run on GTK: the
// Windows and macOS backends have no CSS to load.
var ErrUnsupported = errors.New("gtk: styling is only available on the GTK backend")
