# Changelog

## 0.1.0 — 2026-11-04

首个发布版本：把 DeepSeek Harness 的 Web UI 装进一个 Go + MyGo 的桌面外壳。

### 功能

- 启动 `dsh web`（打包运行时 / 已安装运行时 / PATH 上的 dsh），登录后把工作区
  载入原生窗口。
- 单实例：第二次启动只把已有窗口提到前面。
- 运行时管理：`dsh-go runtime install/use/rollback/off/remove/prune/check/status`，
  菜单里也能检查更新、回滚、切换随包运行时。
- 菜单：File / Edit / View / Server / Window，隐藏菜单栏后 Alt / F10 叫出。
- 窗口状态记忆、全屏、缩放、外部链接交给系统浏览器。

### 渲染与外观

- WebKitGTK 4.1 + GTK 3；在原生 Wayland + Mesa 上自动启用 DMA-BUF 渲染器
  （`DSH_GO_DMABUF` 可强制开关）。
- 跟随桌面深浅色：把设置 portal 的 `color-scheme` 写进 GTK，解决 GTK 3 不读
  GNOME 深色设置的问题。
- 窗口 chrome 用 GTK 自己的 CSS 补齐 GTK 4 观感：12px 圆角、平面顶栏、24px
  圆形标题按钮、隐藏菜单栏。
- 下面两角由页面裁圆（GTK 3 只圆上面两角），透明窗口让桌面透出，最大化时
  自动改回直角。
- 长会话首屏滚动预热：`internal/warmup`，`DSH_GO_WARMUP=0` 关闭。

### 安全

- 日志与错误页诊断里的启动 token 会打码；应用只监听 `127.0.0.1`。

### 已知限制

- Linux 的窗口是 GTK 3，菜单、对话框等原生控件仍是 GTK 3 主题；真正的 GTK 4
  需要另做后端。
- 打包体积大：dsh 依赖树 + Node 约 1.1 GB。
- 端口被占用时改用随机端口，Web UI 的 origin 变化会让界面状态重新开始。
