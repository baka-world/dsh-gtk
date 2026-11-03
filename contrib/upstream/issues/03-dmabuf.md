# DMA-BUF renderer: 2–5× scroll FPS on Wayland, disabled for every app

MyGo sets `WEBKIT_DISABLE_DMABUF_RENDERER=1` for every app (to avoid blank
windows on NVIDIA, VMs and containers). On a plain Wayland + Mesa desktop that
costs most of the scroll performance:

Measured on one 4,586-node chat page, 1600×1000, 12 s of `scrollTop += 24px`
per frame:

| renderer | FPS | WebProcess CPU | UI process CPU |
|---|---|---|---|
| software | 11–19 | 99% (a core) | 24–28% |
| DMA-BUF | 56 | 49% | 48% |

At 2880×1482: 26 vs 57 FPS.

The detection we use before re-enabling it (`DSH_GO_DMABUF=1`):

- `WAYLAND_DISPLAY` names an existing socket, and `GDK_BACKEND` is not `x11`;
- no NVIDIA (`/sys/module/nvidia`, `/proc/driver/nvidia/version`), not a
  container (`/.dockerenv`, `/run/.containerenv`), not a VM (DMI product
  name), and not llvmpipe (`LIBGL_ALWAYS_SOFTWARE`, `GALLIUM_DRIVER`);
- an explicit `WEBKIT_DISABLE_DMABUF_RENDERER` still wins.

Would you take a policy like that as the default (or an opt-in
`AppOption`/`WindowOptions`), keeping the current forced-off behavior for the
hosts that are known to draw blank windows?
