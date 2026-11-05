# 侧边栏收放的帧率

有人反馈侧边栏展开/收起时帧率低。这里记录一次测量：怎么测、测到什么、
以及为什么外壳改不动它。

## 结论

- 侧边栏动画本身是页面的 CSS：`.frame[data-animating]` 上
  `transition: grid-template-columns`，也就是每帧改一遍 grid 轨道宽度，
  整块正文跟着重新布局和重绘。
- 在真实 GPU（Intel UHD 620，2x HiDPI）和一篇 3998 个节点的长会话里，
  开关一次是 **49–52 fps**，空载 60 fps：平均每帧 19.6–20.8 ms，
  最差 82–136 ms。
- 这个开销**不是**本仓库的窗口外观造成的：把窗口换成不透明的
  （`DSH_GTK_TITLEBAR=0`，没有透明、没有四角圆角）结果一样（51/50 fps）；
  在 800x600 窗口里放同一篇会话也一样（52/50 fps），换成 692 个节点的
  轻页面也只有 55–58 fps。
- 每帧的布局成本只有 0.4–1.5 ms，动画期间把正文整个 `visibility:hidden`
  也没有变快（40/47 fps），所以瓶颈不在“重排”也不在“重绘正文”，
  而在页面处理宽度变化的这一步（React 侧随 resize 重算）加上 WebKitGTK
  自己的帧调度。
- 唯一和外壳有关的一条：`DSH_GTK_DMABUF=0` 时收放会掉到 28 fps。
  保持默认的 `auto`/`on`。

顺带记一条：官方 Linux 桌面端（`deepseek-harness`，Rust/Tauri）跑起来拉的是
`webkit2gtk-4.1` 的 `WebKitWebProcess`，没有 Electron 进程——和这里用的是
同一个引擎、同一条渲染路径，所以同一篇会话在官方客户端里也该是同样的数。

结论：这是页面动画的写法问题，自己的外壳没有可调的空间。想真的变快，
要改 dsh 的 web UI（例如动画期间不重排正文，或改成合成器友好的属性），
不是改这里。

## 怎么测的

一次性的测量程序（不进仓库）：在真实的 dsh 页面里预注入脚本，
用 `requestAnimationFrame` 数帧，点同一个按钮，报告每帧间隔；
再以 `grid-template-columns` 走一遍动画的宽度并强制布局，量每帧布局成本。
窗口由 `internal/dsh` + `mygo` 拉起，`DSH_HOME` 指向一份拷出来的
`~/.dsh`（会话在，凭据只在测量期间存在），然后点侧边栏里的长会话。

对照项：

| 变量 | 做法 |
| --- | --- |
| 窗口外观 | `Transparent` + `titlebar.GtkCSS` vs 纯 GTK 3 窗口 |
| 渲染路径 | `DSH_GTK_DMABUF` = `auto` / `off` |
| 像素量 | 1440x732 与 800x600（同为 dpr 2） |
| 正文重绘 | 动画期间给正文加 `visibility:hidden` |
| 轻/重页面 | 新会话 692 节点 vs 长会话 3998 节点 |

注意：没有 `/dev/dri` 时 WebKitGTK 落到 llvmpipe，量出来的帧率没有意义
（长会话里 rAF 甚至一秒钟只出几帧），必须在真实的 GPU 设备节点下测。

## 一次典型输出

```
long conversation, 3998 nodes, 1440x732 @ dpr 2
idle     60 fps   avg 16.7 ms  max 17
collapse 49 fps   avg 20.5 ms  max 136
expand   52 fps   avg 19.6 ms  max 84
layout   0.38–1.54 ms per frame
```

## 现在怎么复测

上面那套探针（`.probe/` 里的窗口 + `Page.Eval`）只是把一个 rAF 计数器塞进页面，
而生产构建恰好没有 inspector，所以同样的数字现在由外壳自己给：
`DSH_GTK_FPS=1 dsh-gtk` 会在页面右上角画 HUD（fps、p50/p95 帧耗时、超出 20 ms
的帧、long task、DOM 节点数、24 秒历史），量的就是这份文档里的 rAF 间隔。

它落在 `PreloadScript` 里（`internal/fps`），只在 http(s) 顶层页面注入，不设
`DSH_GTK_FPS` 就完全不跑。做侧边栏这种对比时，把 HUD 的历史条对着操作看：
展开/收起的那一两秒会直接凹下去，凹到多少就是掉到多少帧。
