# Prepared upstream contributions

Nothing here is published: these are patches and issue drafts for
[egoist/mygo](https://github.com/egoist/mygo), kept for review.

```sh
# apply the patch to a clone of mygo
git clone https://github.com/egoist/mygo
cd mygo && git am ../dsh-go/contrib/upstream/0001-cli-license-from-mygo-json.patch
go test ./cmd/mygo -run 'TestConfigTypes|TestArchLicenseAndURL|TestPKGBUILD'
```

## 0001 — read the PKGBUILD license from mygo.json

`mygo.json` has no `license`, so an app without a `package.json` (native UI,
or a Go app that is not npm-packaged) could only ship `LicenseRef-custom`.
Found while packaging dsh-go, whose PKGBUILD said `LicenseRef-custom` next to
a repository-wide MIT `LICENSE`.

Touches `cmd/mygo/config.go`, `cmd/mygo/pkgbuild.go`, its test, the two
documentation pages and `packages/cli/index.d.ts` (the config types must match
the Go struct).

## issues/ — drafts, not filed

- `01-dark-mode.md` — GTK 3 ignores the settings portal's `color-scheme`, so
  `prefers-color-scheme` stays light on a dark GNOME desktop while
  `Theme.IsDark()` is right. Includes the workaround dsh-go ships and a
  suggested fix in `internal/linux`.
- `02-gtk-css.md` — MyGo has no public GTK CSS hook; apps that want a GTK 4
  looking frame poke `libgtk-3` with purego themselves. Proposes a small
  `Chrome.SetCSS`.
- `03-dmabuf.md` — the DMA-BUF renderer is disabled for every app; on
  Wayland + Mesa that is 11–19 FPS against 56 while scrolling. Includes the
  measurements and the host detection dsh-go uses before re-enabling it.
