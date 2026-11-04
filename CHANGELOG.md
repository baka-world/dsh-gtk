# Changelog

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
