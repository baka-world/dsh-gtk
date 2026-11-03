## Summary

`mygo build` writes the desktop entry without `StartupWMClass`, so an X11 session cannot match an app's window to its entry: the taskbar shows a generic icon, the window cannot be pinned, and the launcher does not mark the app as running. Wayland is unaffected (it matches the app id, which GTK takes from the program name).

Write the key as the executable of the entry — the bare name, before a file association appends `%U` to `Exec`. One line, plus the sentence in `docs/distribution.md` that says what it is for.

Fixes #1128.

## Testing

```sh
go test ./cmd/mygo                                           # ok, 79.9s
go test ./cmd/mygo -run 'TestDesktopEntryStartupWMClass' -v   # ok
```

`TestDesktopEntryStartupWMClass` checks both entry shapes: with and without a file association, whose `%U` must not reach the value. I tested the entry text only — no X11 session here to watch a window group with its launcher.

Tested on:
- [ ] macOS
- [ ] Windows
- [x] Linux
- [ ] Web
- [ ] Other: (specify here)

## AI disclosure

- Tool and model: DeepSeek Harness (`dsh`) with `deepseek-v4.1-flash`
- Used for: the code, the test, and this description
- [ ] A human read every line of this change and can explain it.
