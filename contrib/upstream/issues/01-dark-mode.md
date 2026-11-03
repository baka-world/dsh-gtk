# Linux: a GTK 3 app stays light on a dark desktop, so `prefers-color-scheme` is wrong

## What happens

On a GNOME desktop in dark mode, a page in a MyGo window reports
`prefers-color-scheme: light`, and the GTK widgets MyGo does not draw (menus,
dialogs) follow the light GTK 3 theme. `mygo.Theme.IsDark()` at the same time
returns `true`.

## Why

GTK 3 has no idea about GNOME's `org.gnome.desktop.interface color-scheme`:
the setting lives in the settings portal, and GTK 3 only looks at
`gtk-application-prefer-dark-theme` (which the portal-aware desktops set for
GTK 4 apps). WebKitGTK takes `prefers-color-scheme` from that same GTK
setting, so the media query is light even when the desktop is dark.

MyGo already reads the portal (`portalColorScheme()` in
`internal/linux/system.go`), but it only writes the GTK setting from
`Theme.SetSource`, which an app must call itself.

## Reproduction

A GTK 3 app on GNOME with dark mode, no `Theme.SetSource` call, and a page
logging:

```js
matchMedia('(prefers-color-scheme: dark)').matches // false
```

`mygo.Theme.IsDark()` prints `true`.

## Workaround we ship

```go
mygo.Theme.SetSource(mygo.ThemeSystem)          // before App.Run
mygo.Theme.OnUpdated(followSystemTheme)          // after the portal changes

func followSystemTheme() {
    // SetSource fires OnUpdated again: guard, or it loops.
    if !applying.CompareAndSwap(false, true) { return }
    mygo.Theme.SetSource(mygo.ThemeSystem)
    if mygo.Theme.IsDark() { mygo.Theme.SetSource(mygo.ThemeDark) }
    time.AfterFunc(300*time.Millisecond, func() { applying.Store(false) })
}
```

The `IsDark` follow-up covers a dark GTK theme next to a desktop without a
portal preference.

## Suggested fix

In `internal/linux`, keep `gtk-application-prefer-dark-theme` in step with the
portal while the app has no explicit `ThemeSource` (or has `"system"`):

- once at startup, after GTK is initialized;
- on `SettingChanged` for `org.freedesktop.appearance` / `color-scheme`,
  next to the existing `ThemeChanged()` call.

Then an app that never touches `Theme` still gets a correct media query, and
`SetSource("dark"/"light")` keeps overriding it.
