## What happens

On GNOME in dark mode, a page in a MyGo window reports

```js
matchMedia('(prefers-color-scheme: dark)').matches // false
```

while `mygo.Theme.IsDark()` returns `true` at the same moment. An app that never calls `Theme.SetSource` therefore shows a light page — and a light GTK 3 title bar, menus and dialogs — on a dark desktop.

## Why

GNOME's dark style is `org.gnome.desktop.interface color-scheme`, which lives in the settings portal. GTK 3 does not read it: it looks at `gtk-application-prefer-dark-theme`, and WebKitGTK derives `prefers-color-scheme` from that same GTK setting.

MyGo reads the portal in `theme.IsDark` (`internal/linux/system.go:413`, the `portalColorScheme` call) but writes the GTK setting only from `theme.SetSource` (`internal/linux/system.go:440-461`). Until an app calls it, GTK keeps whatever the theme's default is — light for Adwaita — so pages stay light.

`internal/e2e/color_scheme_test.go` covers the sources an app sets and a theme named for its dark variant, but not a desktop whose preference is only in the portal.

## Workaround we ship

```go
mygo.Theme.SetSource(mygo.ThemeSystem)     // before App.Run
mygo.Theme.OnUpdated(followSystemTheme)    // after a portal change

func followSystemTheme() {
	// SetSource fires OnUpdated again: guard, or it loops.
	if !applying.CompareAndSwap(false, true) {
		return
	}
	mygo.Theme.SetSource(mygo.ThemeSystem)
	if mygo.Theme.IsDark() { // a dark GTK theme next to a desktop without a preference
		mygo.Theme.SetSource(mygo.ThemeDark)
	}
	time.AfterFunc(300*time.Millisecond, func() { applying.Store(false) })
}
```

## Suggested fix

Keep `gtk-application-prefer-dark-theme` in step with the portal while the app has not chosen an appearance (no `SetSource`, or `"system"`):

- once in `Backend.Init`, after `gtkInitCheck`, so the first page already has the right media query;
- in `portalSignal`'s `SettingChanged` branch for `org.freedesktop.appearance` / `color-scheme`, next to the existing `ThemeChanged()` call.

An explicit `Theme.SetSource("dark"/"light")` keeps winning, and the current `IsDark` behavior does not change. The portal path needs a fake `org.freedesktop.portal.Settings` on a private bus to be testable; I am happy to help with that if you want a test before the change.
