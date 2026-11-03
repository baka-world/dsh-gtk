## What we ran into

MyGo's Linux backend is GTK 3. Two things in the frame cannot be finished from an app:

- **The bottom corners.** `decoration { border-radius: 8px 8px 0 0 }` in Adwaita rounds the top two corners; GTK 3 clips the decoration but never clips a window's content, so the page covers the bottom two and they stay square. Verified by drawing a window with `gtk_widget_draw` into a cairo surface and reading the corner alpha: top corners ramp (`17 → 21 → 241 → 255`), bottom corners are `255` immediately, with or without a larger `border-radius`.
- **The header bar.** GTK 3's Adwaita header bar draws a gradient, a border and a 1 px outline; GTK 4's (libadwaita) is flat, and its title buttons are 24 px circles where GTK 3's are 28 px boxes.

Neither can be changed from an app: the CSS provider is internal to MyGo, and there is no hook to add one.

## Workaround we ship

Calling GTK from the app, with purego, at `GTK_STYLE_PROVIDER_PRIORITY_USER`:

```go
libgtk, _ := purego.Dlopen("libgtk-3.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
libgdk, _ := purego.Dlopen("libgdk-3.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)

var addProvider func(screen, provider uintptr, priority uint32)
var providerNew func() uintptr
var loadFromData func(provider uintptr, data *byte, length int, err **gerr) bool
var screenGetDefault func() uintptr
purego.RegisterLibFunc(&addProvider, libgtk, "gtk_style_context_add_provider_for_screen")
purego.RegisterLibFunc(&providerNew, libgtk, "gtk_css_provider_new")
purego.RegisterLibFunc(&loadFromData, libgtk, "gtk_css_provider_load_from_data")
purego.RegisterLibFunc(&screenGetDefault, libgdk, "gdk_screen_get_default")

provider := providerNew()
loadFromData(provider, &css[0], len(css), &gerr)
addProvider(screenGetDefault(), provider, 800)
```

with a sheet such as:

```css
decoration { border-radius: 12px; box-shadow: 0 3px 9px 1px rgba(0, 0, 0, .28); }
.maximized decoration, .fullscreen decoration, .tiled decoration { border-radius: 0; }
.csd.popup decoration, tooltip.csd decoration { border-radius: 5px; }
headerbar { min-height: 46px; padding: 0 6px; background-color: #f9fafb;
            background-image: none; border-width: 0; box-shadow: none; }
button.titlebutton { min-width: 24px; min-height: 24px; margin: 0 3px; padding: 0;
                     border: none; border-radius: 9999px; background-color: transparent; }
```

It works (title buttons measure 24 px afterwards), but every app that wants a GTK 4-looking frame has to repeat the same 40 lines of FFI, on the GTK main thread, with GTK 3's symbols and priorities.

## The ask

A small public surface, for example:

```go
// Load css into GTK's style providers at the user's priority, once the
// toolkit is up (WhenReady). A second call replaces the sheet. It does
// nothing where the backend is not GTK.
func (c *ChromeModule) SetCSS(css string) error
```

Replacing rather than adding keeps a theme switch cheap, and the user priority beats the theme and MyGo's own provider.

If you would rather not have a CSS escape hatch, saying so in the docs would help too: today the alternative is FFI in every app that cares, and the GTK 3 rounding limit is not documented anywhere.
