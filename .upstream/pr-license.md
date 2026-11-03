## Summary

`mygo.json` has no `license`, so `archLicense` could only read `package.json`; an app without one shipped `LicenseRef-custom` even when its repository is MIT — one I maintain did. A `package.json` added only to carry the license also lets npm's field disagree with the app's.

Read `license` from `mygo.json` first, keep `package.json` as the fallback, and document it in `docs/configuration.md`, `docs/distribution.md` and `packages/cli/index.d.ts` (`TestConfigTypes` compares that interface with the Go struct, so the field has to be there).

Fixes #1118.

## Testing

```sh
go test ./cmd/mygo     # ok, 83s
go vet ./cmd/mygo
gofmt -l cmd/mygo packages/cli
```

`TestArchLicenseAndURL` gains three cases: a license from the configuration, its priority over `package.json`, and `UNLICENSED` still mapping to `LicenseRef-custom`.

Tested on:
- [ ] macOS
- [ ] Windows
- [x] Linux
- [ ] Web
- [ ] Other: (specify here)

## AI disclosure

- Tool and model: DeepSeek Harness (`dsh`) with `deepseek-v4.1-flash`
- Used for: the code, the tests, and this description
- [ ] A human read every line of this change and can explain it.
