## What happens

On an X11 session, a MyGo app's window is not associated with its desktop entry: the taskbar shows a generic icon, the window cannot be pinned, and the launcher does not mark the app as running. Wayland is fine, because it matches the wayland app id, which GTK takes from the program name.

## Why

The desktop entry `mygo build` writes (`linuxDesktopEntry` in `cmd/mygo/build.go`) has `Type`, `Name`, `Comment`, `Exec`, `Icon`, `Categories`, `Terminal` and, for file associations, `MimeType` — but no `StartupWMClass`. An X11 window manager matches a window to its entry by WM_CLASS, which GTK sets from the program name, the executable the entry runs; without the key the association is left to guesswork.

## The ask

Write `StartupWMClass=<executable>`, the command's name (the slug), next to `Exec`. It is one line, it cannot hurt an app that does not need it (the value is the name `Exec` already runs), and it makes the entry complete for X11 sessions.

I have a patch: the key in `linuxDesktopEntry`, a test that the value stays the bare name when a file association appends `%U` to `Exec`, and a sentence in `docs/distribution.md`. Happy to open it as a PR; naming is yours to change.
