`archLicense` (`cmd/mygo/pkgbuild.go`) reads the SPDX expression from `package.json`, and falls back to `LicenseRef-custom` when there is none:

```go
license := strings.TrimSuffix(strings.TrimPrefix(packageJSONString(c.root, "license"), "("), ")")
```

So an app with a frontend gets its license into the PKGBUILD, while an app without one — native UI, or a Go app that is not npm-packaged — cannot say what it is even when the repository carries a `LICENSE` at its root. Adding a `package.json` only for `license` also invites npm's field and the app's own to drift apart: a repository can be MIT and its frontend `SEE LICENSE IN …`.

A concrete case: a desktop app I maintain is MIT (`LICENSE` at the root, `package.json` only for tooling) and its PKGBUILD said `license=('LicenseRef-custom')`.

**The ask:** a `license` field in `mygo.json` that `archLicense` prefers, with `package.json` as the fallback, documented in `docs/configuration.md` and `docs/distribution.md` and in the configuration types (`packages/cli/index.d.ts`).

I have a patch for this — `cmd/mygo/config.go`, `cmd/mygo/pkgbuild.go`, `TestArchLicenseAndURL`, the two documentation pages, and the TypeScript interface — and I am opening it as a PR; naming and scope are easy to change if you would rather take another route.
