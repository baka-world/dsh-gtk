# Changelog

## 0.1.9 — 2026-11-06

- 新增 `DSH_GTK_FPS_LOG=1`（配合 `DSH_GTK_FPS`）：外壳每秒用 `Page.Eval` 读一次
  页面里的 `window.__dshFPS.report()`，把数字写进自己的日志：

  ```
  dsh-gtk: fps fps=60 nodes=15666 mutPeak=204 slow=1229ms,61ms
  ```

  这样一次测量就是**一个文件**（`dsh-gtk > run.log 2>&1`），可以 grep、可以看
  时间序列，不用每次截图。`slow` 是历史最慢的三帧，`mutPeak` 是 DOM 增删峰值。

## 0.1.8 — 2026-11-06

- 新增 `DSH_GTK_CSS=/path/to/x.css`：把这个文件作为 `<style>` 注入每个页面。为了
  调侧边栏的布局成本，需要反复试「禁用过渡」「钉住列宽」「给消息加 contain」这些
  CSS，每次重新编译要十几分钟；有了这个开关，改文件 + 重启应用就能看到效果。

  不是给用户调主题用的（主题走 GTK），是一个实验/诊断入口。

## 0.1.7 — 2026-11-06

- 帧率 HUD 换了两行指标，因为它要回答的问题变了：**一次侧边栏开合到底是
  「一帧特别长」还是「每帧都慢」**。

  - 新 `slow` 行：有史以来最慢的三帧及其发生时间。实测长会话（15,666 个节点）
    上，一次开合只产生**一帧 1274 ms**，其余帧都是 17 ms——所以「最低 fps 1」
    是那一帧把 1 秒窗口里只剩 1 帧，而不是持续掉帧。
  - 新 `mut` 行：最近一秒 DOM 增删的节点数（含峰值）。它把两种可能分开：
    React 重渲染会增删成千上万个节点，纯 reflow 一个都不增。
  - 删掉 `lt`（long task）：WebKit 不支持 `longtask` 条目，那一行永远是 0，是
    死指标。

## 0.1.6 — 2026-11-06

- 端口不再靠「猜」：外壳先自己 `bind` 127.0.0.1:3081，把监听套接字作为 fd 3 交给
  dsh，再用 `--require` 预载一段脚本（`internal/dsh/listenfd.js`），让 webserver
  插件直接 adopt 这个 fd，而不是自己 bind。于是：

  - dsh 不可能 bind 失败，也没有「挑完端口到 dsh 真正 listen 之间被别人抢」的窗口；
  - 端口固定 → origin 固定 → `localStorage` 里的界面状态（侧边栏宽度、当前会话）
    能留住；
  - 不再有启动时那次注定失败的尝试。

  预载脚本用 argv 传（`node --require …`）而不是 `NODE_OPTIONS`：用户数据目录名
  带空格（`DSH GTK`），argv 不需要转义。`--dsh` 指向独立可执行文件时这条路自动
  跳过，回到 dsh 自己 bind。

- 用真实运行时验证过：在 3099 上 bind 一个套接字交给 dsh 0.2.2-rc.1，它报出来的
  是 `dsh web: http://127.0.0.1:3099/…`，即 adopt 的套接字而不是新 bind 的。

## 0.1.5 — 2026-11-05

- 修「每次启动都先卡 17 秒、界面状态像被重置」：应用默认端口是 3080，而常驻的
  `dsh web`（比如你自己跑的 `dsh-web-serve`）会一直占着它，于是每次启动都先起一个
  必定失败的 server（日志里 `port 3080 is taken`，约 17 秒），再退到随机端口。
  随机端口意味着 origin 变了，而 web UI 的 localStorage（侧边栏宽度、选中的会话）
  是按 origin 存的——所以每次重启都像被清空了一样。

  现在启动前先用 `net.Listen` 探一遍，从 3080 往上取第一个空的（你机器上是 3081），
  每次都是同一个，既省掉那 17 秒，也让界面状态能留住。

## 0.1.4 — 2026-11-05

- 换运行时后不用再猜为什么没生效。`dsh-gtk runtime install` 只写选择文件，dsh 是
  应用的子进程，不重启就一直跑旧的——装完点 Later 之后就再没人提醒。现在外壳每
  2 秒对一次选择与正在跑的树，不一致就弹一次 **Runtime Changed**（Restart /
  Later）；选回正在跑的那个之后不提示，再换又会提示。`DSH_GTK_DSH` 或 PATH 里的
  dsh 不受选择影响，不参与这个提醒。

- 更新检测不再固定跟 `latest`：它比随包的 `0.2.2-alpha.3` 还旧（`0.2.1-rc.5`），
  于是「Check Runtime Updates」永远回答 up to date。现在默认跟**当前运行时自己
  所在的 channel**——先找版本号相同的 tag，找不到就跟版本号最大的那个；dsh 的
  预发布挂在 `next`/`alpha` 上，这样 `0.2.2-rc.1` 才看得见。`DSH_GTK_CHANNEL`
  仍可强制指定，`runtime install` 不带参数时装的也是这个 channel 的版本，而不是
  更旧的 `latest`。

- 新增 `DSH_GTK_FPS=1`：页面右上角的帧率 HUD（fps、p50/p95 帧耗时、超预算帧数、
  long task、DOM 节点数、24 秒历史）。生产构建没有 WebKit inspector，帧率从进程
  外量不到，这个是在页面里用 `requestAnimationFrame` 量的；`Alt+Shift+F` 隐藏。
  也就是上面那轮侧边栏测量的工具化版本，见 README「帧率 HUD」。

## 0.1.3 — 2026-11-04

- 修「装好的应用里的 dsh 起不来」：`mygo build` 复制 `resources/` 时跳过所有点
  开头的名字，运行时就丢了 36 个点文件，其中
  `@earendil-works/pi-ai/dist/providers/data/.manifest.json` 是 `llm-pi-ai` 插件
  静态 import 的，缺了它 dsh 直接 `startup failed: 2 required plugins did not
  activate`，窗口停在错误页。现在打包产物里 36 个点文件全在（`dist` 里核对过）。

- 依赖改走 fork：[baka-world/mygo](https://github.com/baka-world/mygo) 的 `main`
  = 上游 main + 我们的补丁，fork 里的 `.github/workflows/sync-upstream.yml` 每
  6 小时合并一次上游并跑 `go test ./cmd/mygo`，冲突就失败等人处理。这样上面那个
  修复和 `Theme.SetGTKCSS` 之类不用等上游发版。

- 修托盘/菜单里的 Quit 退不掉应用：`mygo.App.Quit()` 是先关窗口再触发
  `OnWillQuit`，而有托盘时我们关窗只是隐藏，于是退出被自己否决，Quit、⌘Q、
  SIGTERM 全都无效。现在在 `OnBeforeQuit`（关窗之前）就标记退出。实测：修之前
  `kill -TERM` 后进程还在，修之后立刻退出。

## 0.1.2 — 2026-11-04

修 0.1.1 的 RPM：`/usr/bin/dsh-gtk` 是 `../opt/dsh-gtk/dsh-gtk`，从 `/usr/bin` 出发
会解析成 `/usr/opt/…`，断链。后果不止是命令跑不起来——GNOME 和 GIO 会因为这个
`Exec` 找不到而**整条 desktop 条目都不加载**，于是应用菜单里没有图标，装完像什么
都没发生。

- 链接改成 `../../opt/dsh-gtk/dsh-gtk`。
- 加了 `TestRPMLinkResolves`：按 `mygo.json` 的 slug 把 spec 里的 `ln -s` 目标拼成
  绝对路径，链接指错就红。旧的 spec 会失败，修好的会通过。

`.deb`、`.tar.gz` 和 `install.sh` 不受影响（那条链接由 `mygo build` 写成
`../../opt/…`，本来就是对的）。其他发行版用 `.deb`/`.tar.gz` 的不用升级。

## 0.1.1 — 2026-11-04

- 桌面集成：常驻托盘图标（显示窗口 / 退出），RPM 的 desktop 入口补上
  `StartupWMClass`、`Keywords`、`GenericName`、`SingleMainWindow`，安装后能像
  普通桌面应用一样出现在应用菜单里、窗口和入口归组、任务栏显示图标。桌面没有
  托盘宿主时只是日志里记一句，关窗即退出，和之前一样。

- 跟随 MyGo 的新接口，删掉了外壳里的三段 workaround：
  - `App.SetDMABUFRenderer`（Wayland + Mesa 上自动启用 DMA-BUF，NVIDIA/虚拟机/
    容器保持关闭）取代自带的 90 行宿主机探测；
  - MyGo 自己同步设置 portal 的深色风格，取代 `SetSource` 防重入那套；
  - `Theme.SetGTKCSS` 取代 `internal/gtk` 里 100 行 purego FFI（该包已删除）。

- 打包元数据收进 `mygo.json`：SPDX 许可证写成 `license` 字段，RPM 和 PKGBUILD
  都从这里读，`package.json` 因此删掉了（上游 mygo#1119 已合并）。

- 依赖固定到 MyGo 的 main（`v0.7.5-0.20261103195256-8871e2f469fa`）：上面的新
  接口还没进 tag。v0.7.5 发布后改回 tag，再发一版。

- 文档：补上侧边栏收放的帧率测量（`docs/perf-sidebar.md`），并说明为什么那不是
  外壳能调的。

## 0.1.0 — 2026-11-04

首个发布版本：把 DeepSeek Harness 的 Web UI 装进一个 Go + MyGo 的桌面外壳。

### 功能

- 启动 `dsh web`（打包运行时 / 已安装运行时 / PATH 上的 dsh），登录后把工作区
  载入原生窗口。
- 单实例：第二次启动只把已有窗口提到前面。
- 运行时管理：`dsh-gtk runtime install/use/rollback/off/remove/prune/check/status`，
  菜单里也能检查更新、回滚、切换随包运行时。
- 菜单：File / Edit / View / Server / Window，隐藏菜单栏后 Alt / F10 叫出。
- 窗口状态记忆、全屏、缩放、外部链接交给系统浏览器。

### 渲染与外观

- WebKitGTK 4.1 + GTK 3；在原生 Wayland + Mesa 上自动启用 DMA-BUF 渲染器
  （`DSH_GTK_DMABUF` 可强制开关）。
- 跟随桌面深浅色：把设置 portal 的 `color-scheme` 写进 GTK，解决 GTK 3 不读
  GNOME 深色设置的问题。
- 窗口 chrome 用 GTK 自己的 CSS 补齐 GTK 4 观感：12px 圆角、平面顶栏、24px
  圆形标题按钮、隐藏菜单栏。
- 下面两角由页面裁圆（GTK 3 只圆上面两角），透明窗口让桌面透出，最大化时
  自动改回直角。
- 长会话首屏滚动预热：`internal/warmup`，`DSH_GTK_WARMUP=0` 关闭。

### 打包

- `make build` 出 `.deb`、`.tar.gz` 和 `install.sh`；`make rpm` 再出一个 RPM
  （应用与运行时在 `/opt/dsh-gtk`，命令链接到 `/usr/bin/dsh-gtk`）。

### 安全

- 日志与错误页诊断里的启动 token 会打码；应用只监听 `127.0.0.1`。

### 已知限制

- Linux 的窗口是 GTK 3，菜单、对话框等原生控件仍是 GTK 3 主题；真正的 GTK 4
  需要另做后端。
- 打包体积大：dsh 依赖树 + Node 约 1.1 GB。
- 端口被占用时改用随机端口，Web UI 的 origin 变化会让界面状态重新开始。
