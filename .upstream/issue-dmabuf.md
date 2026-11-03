## What happens

`Backend.Init` sets `WEBKIT_DISABLE_DMABUF_RENDERER=1` for every app unless the environment already has it (`internal/linux/app.go:115-119`), to avoid blank windows on NVIDIA, VMs and containers. On a plain Wayland + Mesa desktop that costs most of the scroll performance.

Measured with the same page (a 4,586-node chat transcript), 1600×1000, 12 s of `scrollTop += 24px` per frame, WebKitGTK 2.54.2, Intel UHD 620:

| renderer | FPS | WebProcess CPU | UI process CPU |
|---|---|---|---|
| software (what MyGo forces) | 11–19 | 99% (a core) | 24–28% |
| DMA-BUF | 56 | 49% | 48% |

At 2880×1482: 26 FPS against 57.

The same page also reproduces the known trade-off: with software rendering the process is pinned to a core while scrolling, which matters on laptops.

## What we do instead

Decide before the toolkit starts, then let WebKit have the renderer when the host is known to be safe:

- `WAYLAND_DISPLAY` names an existing socket (a stale variable does not count) and `GDK_BACKEND` is not `x11`;
- no NVIDIA (`/sys/module/nvidia`, `/proc/driver/nvidia/version`), not a container (`/.dockerenv`, `/run/.containerenv`), not a VM (DMI `product_name` against virtualbox/vmware/qemu/kvm/bochs/parallels/xen/hyper-v), and not llvmpipe (`LIBGL_ALWAYS_SOFTWARE`, `GALLIUM_DRIVER=llvmpipe`);
- an explicit `WEBKIT_DISABLE_DMABUF_RENDERER` from the caller always wins.

## The ask

Any of these would help; the first is what we would use:

1. an opt-in, such as `AppOptions`/`WindowOptions` (or `mygo.GPU(...)`) to keep the renderer on;
2. the policy above as the default on Wayland, keeping the current forced-off behavior for the hosts that draw blank windows;
3. or documentation of the environment variable and its cost, if you would rather leave the choice to apps.

I am not proposing to remove the safety default: the blank windows are the reason it exists. But "Wayland, Mesa, no NVIDIA, not a container" is a large share of Linux desktops, and there the default halves the frame rate of any scrolling page.
