# Expose a GTK CSS hook so an app can finish the window chrome

## What we ran into

MyGo's Linux backend is GTK 3. A GTK 3 window rounds only its top corners
(`decoration { border-radius }` is applied to the shadow, the content is never
clipped): the bottom two corners stay square, and GTK 3's Adwaita header bar
has a gradient, a border and a 1px outline that GTK 4 dropped. There is no
public way to change either, because the CSS provider is internal to MyGo.

We ended up calling GTK ourselves, with purego, from the app:

```go
libgtk, _ := purego.Dlopen("libgtk-3.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
libgdk, _ := purego.Dlopen("libgdk-3.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
purego.RegisterLibFunc(&addProvider, libgtk, "gtk_style_context_add_provider_for_screen")
purego.RegisterLibFunc(&providerNew, libgtk, "gtk_css_provider_new")
purego.RegisterLibFunc(&loadFromData, libgtk, "gtk_css_provider_load_from_data")
purego.RegisterLibFunc(&screenGetDefault, libgdk, "gdk_screen_get_default")

provider := providerNew()
loadFromData(provider, &css[0], len(css), &gerr)
addProvider(screenGetDefault(), provider, 800) // GTK_STYLE_PROVIDER_PRIORITY_USER
```

With that, `decoration { border-radius: 12px }` and libadwaita's 24px
`button.titlebutton` circles do apply, so it works — but every app that wants a
GTK 4-looking frame has to repeat it, on the GTK main thread, with GTK 3's
symbols and priorities.

## What we would like

A public, small surface, for example:

```go
// MyGo loads css into GTK's style providers at the user's priority, after
// the toolkit is up (WhenReady). A second call replaces the sheet. It does
// nothing where the backend is not GTK.
func (c *ChromeModule) SetCSS(css string) error
```

- loaded once, not per window, so reloading on theme changes is cheap;
- `GTK_STYLE_PROVIDER_PRIORITY_USER`, so it beats the theme and MyGo's own
  provider;
- documented as GTK-only (a no-op on macOS/Windows, where the platform draws
  the frame).

The window corner case would be worth documenting too: GTK 3 clips only the
top corners, so a page that rounds its own bottom corners needs a transparent
window (`WindowOptions.Transparent`), which is what we do.
