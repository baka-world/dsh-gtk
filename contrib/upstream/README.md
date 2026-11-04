# Upstream contributions to MyGo

What dsh-gtk ran into, and what was filed upstream on 2026-11-04.

## Filed

| # | Kind | What |
|---|---|---|
| [#1118](https://github.com/egoist/mygo/issues/1118) | issue | `mygo.json` has no `license`, so the PKGBUILD can only say `LicenseRef-custom` |
| [#1119](https://github.com/egoist/mygo/pull/1119) | PR, **merged** | `fix(cli): read the PKGBUILD license from mygo.json` — fixes #1118, merged as 03e6ccc1 |
| [#1120](https://github.com/egoist/mygo/issues/1120) | issue | GTK 3 stays light on a dark desktop, so `prefers-color-scheme` is wrong |
| [#1121](https://github.com/egoist/mygo/issues/1121) | issue | no public way to style GTK's own widgets (the FFI workaround) |
| [#1122](https://github.com/egoist/mygo/issues/1122) | issue | the DMA-BUF renderer is forced off, costing 2–5× scroll FPS on Wayland + Mesa |
| [#1128](https://github.com/egoist/mygo/issues/1128) | issue | the generated desktop entry has no `StartupWMClass`, so X11 cannot group the window with it |
| [#1129](https://github.com/egoist/mygo/pull/1129) | PR, **merged** | `cli: name the window class in the desktop entry` — fixes #1128, merged as 4d10c37 |

Both PRs fill the template's Summary / Testing / AI disclosure sections. Its
"A human read every line" box is left unchecked on purpose: check it once you
have read the diff.

#1119 and #1129 are merged, but the newest MyGo tag is still v0.7.4, which
predates both. The license has already moved to `mygo.json` here — `mygo build`
reads it from the working tree — so this repository only waits on a tag to drop
the pseudo-version in `go.mod`. The desktop entry still comes from `mygo build`
for `.deb` and `.tar.gz`; the RPM ships its own, richer one.
