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

The PR fills the template's Summary / Testing / AI disclosure sections. Its
"A human read every line" box is left unchecked on purpose: check it once you
have read the diff.

The PR is merged, but the newest MyGo tag is still v0.7.4, which predates it.
Once a release carries the change, this repository can move its license from
`package.json` to `mygo.json` and drop `package.json`, which exists only for
that field.

## The patch

`0001-cli-license-from-mygo-json.patch` is what #1119 carries, kept here so a
rebase is one `git am` away:

```sh
git clone https://github.com/egoist/mygo
cd mygo
git am ../dsh-gtk/contrib/upstream/0001-cli-license-from-mygo-json.patch
go test ./cmd/mygo
```

Touches `cmd/mygo/config.go`, `cmd/mygo/pkgbuild.go`, `TestArchLicenseAndURL`,
`docs/configuration.md`, `docs/distribution.md` and `packages/cli/index.d.ts`
(the config types must match the Go struct: `TestConfigTypes`).

## Still open upstream, but not filed

- Nothing: the four findings we had are filed. If any of the three issues is
  accepted, the workaround in this repository can be dropped:
  - #1120 → `followSystemTheme` in `main.go` becomes unnecessary;
  - #1121 → `internal/gtk` becomes one call;
  - #1122 → the renderer policy in `configureWebRenderer` becomes redundant.
