// Command dsh-go is a MyGo desktop shell for DeepSeek Harness: it starts
// the dsh web server and shows the workspace in a native window.
//
//	go run .
//	go tool mygo build
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/baka-world/dsh-go/internal/dsh"
	"github.com/baka-world/dsh-go/internal/gtk"
	dshruntime "github.com/baka-world/dsh-go/internal/runtime"
	"github.com/baka-world/dsh-go/internal/titlebar"
	"github.com/baka-world/dsh-go/internal/warmup"
	"github.com/egoist/mygo"
)

//go:embed web
var webFiles embed.FS

const appName = "DSH Go"

// config is how the shell starts dsh, from the command line and the
// environment.
type config struct {
	workspace string
	home      string
	port      int
	dsh       string
	node      string
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "runtime" {
		os.Exit(runRuntimeCommand(os.Args[2:]))
	}
	configureWebRenderer()
	if warmup.Enabled() {
		log.Printf("dsh-go: scroll warm-up: on")
	} else {
		log.Printf("dsh-go: scroll warm-up: off (DSH_GO_WARMUP)")
	}
	if titlebar.Enabled() {
		log.Printf("dsh-go: title bar: drawn by the page (DSH_GO_TITLEBAR=0 keeps GTK 3's)")
	} else {
		log.Printf("dsh-go: title bar: GTK 3 (DSH_GO_TITLEBAR)")
	}
	cfg := parseFlags()

	// A second launch brings the running app forward instead of starting a
	// second server.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.SetName(appName)

	l := newLauncher(cfg)
	mygo.SetFrontend(webFS())
	mygo.Bind(l)
	mygo.App.SetMenu(l.menu())

	// GTK 3 reports a light appearance to WebKit unless
	// gtk-application-prefer-dark-theme says otherwise, and GNOME's dark
	// style lives in the settings portal, which GTK 3 ignores. Read it into
	// GTK so that prefers-color-scheme — and with it dsh's "system" theme —
	// follows the desktop. SetSource alone works before Run; the follow-up
	// read of the appearance waits for it.
	mygo.Theme.SetSource(mygo.ThemeSystem)
	mygo.Theme.OnUpdated(followSystemTheme)
	mygo.App.OnSecondInstance(func([]string, string) { l.openWindow() })

	mygo.App.WhenReady(func() {
		// The appearance drives the window chrome below, so it goes first.
		followSystemTheme()
		log.Printf("dsh-go: appearance: %s", appearance())
		l.openWindow()
		go l.start()
	})

	// Quitting stops the server: an agent must not outlive the window that
	// shows it.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { l.terminate() })
	mygo.App.OnQuit(func() { l.shutdown() })

	// Keep the macOS convention of a running app with no window, and reopen
	// it from the Dock.
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" {
			mygo.App.Quit()
		}
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if runtime.GOOS == "darwin" && !hasVisibleWindows {
			l.openWindow()
		}
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// themeApplying keeps the appearance the app gives GTK from coming back as
// a change of the desktop's.
var themeApplying atomic.Bool

// followSystemTheme gives GTK 3 the desktop's appearance, which only the
// settings portal carries: GNOME's dark style is
// org.gnome.desktop.interface color-scheme, which GTK 3 does not read, so a
// GTK 3 app stays light in a dark session. WebKitGTK takes
// prefers-color-scheme from GTK's gtk-application-prefer-dark-theme, and
// dsh resolves its "system" theme from that media query, so without this
// both the page-drawn title bar and the workspace stay light.
//
// MyGo's SetSource("system") writes exactly that GTK setting from the
// portal; the second step covers a dark GTK theme next to a desktop without
// a portal preference, where the theme name is all GTK has to go on.
func followSystemTheme() {
	if !themeApplying.CompareAndSwap(false, true) {
		return
	}
	mygo.Theme.SetSource(mygo.ThemeSystem)
	if mygo.Theme.IsDark() {
		mygo.Theme.SetSource(mygo.ThemeDark)
	}
	if titlebar.Enabled() {
		// The window chrome, in GTK's own CSS (internal/titlebar): the
		// corners, the flat header bar and the title buttons, which MyGo's
		// API stops short of. It has to be the GTK main thread.
		if err := gtk.Style(titlebar.GtkCSS(mygo.Theme.IsDark())); err != nil {
			log.Printf("dsh-go: window chrome: %v", err)
		}
	}
	time.AfterFunc(300*time.Millisecond, func() { themeApplying.Store(false) })
}

// configureWebRenderer picks WebKitGTK's renderer before the toolkit starts.
//
// MyGo disables WebKitGTK's DMA-BUF renderer for every app, because it draws
// blank windows on NVIDIA, virtual machines and containers. On Wayland with a
// Mesa driver it instead composites on the GPU, and scrolling a large dsh
// conversation is 2-5x faster: measured with the same 4,586-node chat,
// 1600x1000, a 12 s continuous scroll ran at 11-19 fps on the software
// renderer (WebProcess pinned at 99% of a core) and 56 fps with DMA-BUF.
//
// DSH_GO_DMABUF=1 forces it on, DSH_GO_DMABUF=0 forces it off. A
// WEBKIT_DISABLE_DMABUF_RENDERER the caller set is always respected.
func configureWebRenderer() {
	if os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") != "" {
		return
	}
	disable, reason := rendererChoice(
		os.Getenv("DSH_GO_DMABUF"),
		waylandAvailable(),
		strings.EqualFold(os.Getenv("GDK_BACKEND"), "x11"),
		blankRendererHost(),
	)
	if disable {
		_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	} else {
		_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "0")
	}
	log.Printf("dsh-go: web renderer: %s", reason)
}

// rendererChoice is the policy of configureWebRenderer, without the side
// effects.
func rendererChoice(override string, wayland, gdkX11, blankHost bool) (disable bool, reason string) {
	switch strings.ToLower(strings.TrimSpace(override)) {
	case "1", "on", "true", "dma", "dmabuf":
		return false, "dma-buf (DSH_GO_DMABUF)"
	case "0", "off", "false", "software":
		return true, "software (DSH_GO_DMABUF)"
	}
	if !wayland || gdkX11 {
		return true, "software (not Wayland)"
	}
	if blankHost {
		return true, "software (NVIDIA, VM or container)"
	}
	return false, "dma-buf (Wayland)"
}

// waylandAvailable reports whether WAYLAND_DISPLAY names a socket that
// exists, so that a stale variable does not send an X11 window down the
// DMA-BUF path.
func waylandAvailable() bool {
	name := os.Getenv("WAYLAND_DISPLAY")
	if name == "" {
		return false
	}
	if filepath.IsAbs(name) {
		_, err := os.Stat(name)
		return err == nil
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// blankRendererHost reports hosts where WebKitGTK's DMA-BUF renderer draws
// blank webviews: NVIDIA drivers, virtual machines and containers.
func blankRendererHost() bool {
	for _, p := range []string{"/sys/module/nvidia", "/proc/driver/nvidia/version", "/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	if os.Getenv("LIBGL_ALWAYS_SOFTWARE") != "" || strings.Contains(os.Getenv("GALLIUM_DRIVER"), "llvmpipe") {
		return true
	}
	if b, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
		name := strings.ToLower(string(b))
		for _, marker := range []string{"virtualbox", "vmware", "qemu", "kvm", "bochs", "parallels", "xen", "hyper-v"} {
			if strings.Contains(name, marker) {
				return true
			}
		}
	}
	return false
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.workspace, "workspace", os.Getenv("DSH_GO_WORKSPACE"), "directory dsh works in (default: your home directory)")
	flag.StringVar(&cfg.home, "home", os.Getenv("DSH_GO_HOME"), "DSH_HOME of the server (default: dsh's own, ~/.dsh)")
	flag.IntVar(&cfg.port, "port", 3080, "preferred port of the web server; 0 picks a free one")
	flag.StringVar(&cfg.dsh, "dsh", "", "dsh executable or lib/bin.js (default: bundled runtime, then PATH)")
	flag.StringVar(&cfg.node, "node", "", "node executable for a JavaScript dsh entry point")
	flag.Parse()
	if cfg.workspace == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.workspace = home
		}
	}
	return cfg
}

// Launcher owns the window and the dsh server, and is bound for the splash
// and error pages.
type Launcher struct {
	cfg config

	mu       sync.Mutex
	win      *mygo.Window
	srv      *dsh.Server
	gen      int
	starting bool
	lastErr  string

	logFile    *os.File
	logPath    string
	ctx        context.Context
	cancel     context.CancelFunc
	installing bool
}

func newLauncher(cfg config) *Launcher {
	l := &Launcher{cfg: cfg}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	if dir, err := mygo.App.Path(mygo.PathLogs); err == nil {
		if f, err := os.OpenFile(filepath.Join(dir, "dsh.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			l.logFile = f
			l.logPath = f.Name()
		}
	}
	return l
}

// openWindow shows the window, creating it when it does not exist yet.
func (l *Launcher) openWindow() {
	l.mu.Lock()
	if w := l.win; w != nil && !w.IsDestroyed() {
		l.mu.Unlock()
		w.Restore()
		w.Show()
		w.Focus()
		return
	}
	srv := l.srv
	gen := l.gen
	url := "/splash.html"
	if srv == nil && l.lastErr != "" {
		url = "/error.html"
	}
	opts := mygo.WindowOptions{
		Title:           appName,
		Width:           1280,
		Height:          860,
		MinWidth:        720,
		MinHeight:       480,
		URL:             url,
		StateKey:        "main",
		BackgroundColor: "light-dark(#f9fafb, #1b1b1c)",
		// GTK's own title bar, as the official desktop app keeps it on Linux:
		// the frame, its title (the page's) and its buttons are the desktop's.
		// The menu bar stays out of sight, and Alt or F10 reveals it.
		AutoHideMenuBar: true,
		// GTK 3 never clips a window's content to its corner radius, so the
		// page rounds its own bottom corners and shows the desktop through
		// the window beneath them (see internal/titlebar).
		Transparent: titlebar.Enabled(),
		Page: mygo.PageOptions{
			DevTools:        mygo.DevToolsAuto,
			UserAgentSuffix: userAgentSuffix(),
			PreloadScript:   preloadScript(),
		},
	}
	l.win = mygo.NewWindow(opts)
	w := l.win
	neverStarted := l.gen == 0 && !l.starting
	l.mu.Unlock()

	w.OnClosed(func() {
		l.mu.Lock()
		if l.win == w {
			l.win = nil
		}
		l.mu.Unlock()
	})
	if titlebar.Enabled() {
		round := func(rounded bool) {
			_, _ = w.Page().Eval(titlebar.RoundedScript(rounded))
		}
		w.OnMaximize(func() { round(false) })
		w.OnUnmaximize(func() { round(true) })
		w.OnEnterFullScreen(func() { round(false) })
		w.OnLeaveFullScreen(func() { round(true) })
		// A reload starts a square page: give it the window's rounding.
		w.Page().OnDOMReady(func() {
			if !w.IsMaximized() {
				round(true)
			}
		})
	}
	switch {
	case srv != nil:
		// The window shares the app's cookie store: sign it in and load
		// the workspace, a new window or the one the user closed.
		go l.showWorkspace(srv, gen)
	case neverStarted:
		go l.start()
	}
}

// start stops the previous server, starts a new one, and shows it.
func (l *Launcher) start() {
	l.mu.Lock()
	if l.starting {
		l.mu.Unlock()
		return
	}
	l.gen++
	gen := l.gen
	l.starting = true
	l.lastErr = ""
	old := l.srv
	l.srv = nil
	l.mu.Unlock()

	if old != nil {
		_ = old.Stop(5 * time.Second)
	}
	l.showSplash()

	srv, err := l.boot(l.cfg.port)
	if err != nil && l.cfg.port != 0 && errors.Is(err, dsh.ErrAddrInUse) {
		// Another dsh owns the preferred port: take a free one. The origin
		// changes with it, and so does the web UI's stored state.
		log.Printf("dsh-go: port %d is taken; asking for a free one", l.cfg.port)
		srv, err = l.boot(0)
	}

	l.mu.Lock()
	if l.gen != gen {
		l.mu.Unlock()
		if srv != nil {
			_ = srv.Stop(2 * time.Second)
		}
		return
	}
	l.starting = false
	if err != nil {
		l.lastErr = err.Error()
		l.mu.Unlock()
		log.Printf("dsh-go: %v", err)
		l.showError()
		return
	}
	l.srv = srv
	l.mu.Unlock()

	l.showWorkspace(srv, gen)
	go l.watch(srv, gen)
}

func (l *Launcher) boot(port int) (*dsh.Server, error) {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return nil, fmt.Errorf("find the app resources: %w", err)
	}
	installedDir, installedNode := installedRuntime()
	launch, err := dsh.Discover(dsh.DiscoverOptions{
		Resources:     resources,
		InstalledDir:  installedDir,
		InstalledNode: installedNode,
		DSH:           l.cfg.dsh,
		Node:          l.cfg.node,
	})
	if err != nil {
		return nil, err
	}
	log.Printf("dsh-go: %s runtime: %s", launch.Source, launch.Executable)
	var w io.Writer = os.Stderr
	if l.logFile != nil {
		w = l.logFile
	}
	return dsh.Start(dsh.Config{
		Launch:       launch,
		Workspace:    l.cfg.workspace,
		Home:         l.cfg.home,
		Port:         port,
		Log:          w,
		ReadyTimeout: 3 * time.Minute,
		Ctx:          l.ctx,
	})
}

// watch reports a server that stops without being asked to.
func (l *Launcher) watch(srv *dsh.Server, gen int) {
	<-srv.Done()

	l.mu.Lock()
	current := l.gen == gen && l.srv == srv
	l.mu.Unlock()
	if !current || srv.Stopping() {
		return
	}

	err := srv.Err()
	if err == nil {
		err = errors.New("the dsh server exited")
	}
	l.mu.Lock()
	if l.srv == srv {
		l.srv = nil
	}
	l.lastErr = err.Error()
	l.mu.Unlock()
	l.showError()
}

// showWorkspace signs the window in and loads the workspace: the launch
// token is exchanged for the server's cookie here, because a redirect that
// starts on the app's own scheme is a cross-site navigation, and the
// server's SameSite=Strict cookie would be withheld from it.
func (l *Launcher) showWorkspace(srv *dsh.Server, gen int) {
	cookies, err := srv.AuthCookies(15 * time.Second)
	if err != nil {
		l.fail(err)
		return
	}
	origin := srv.Origin()
	host := "127.0.0.1"
	if u, err := url.Parse(origin); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	// A previous launch left cookies of its own server key under this
	// origin; they only pile up in the session's store.
	if stale, err := mygo.Cookies.List(mygo.CookieFilter{URL: origin}); err == nil {
		for _, c := range stale {
			if strings.HasPrefix(c.Name, "dsh-auth-") {
				_ = mygo.Cookies.Delete(c)
			}
		}
	}
	l.mu.Lock()
	current := l.gen == gen && l.srv == srv
	l.mu.Unlock()
	if !current {
		return
	}
	for _, c := range cookies {
		expires := c.Expires
		if expires.IsZero() && c.MaxAge > 0 {
			expires = time.Now().Add(time.Duration(c.MaxAge) * time.Second)
		}
		if err := mygo.Cookies.Set(mygo.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   host,
			Path:     "/",
			Expires:  expires,
			HTTPOnly: true,
			// Lax still goes with the top-level navigation into the page;
			// Strict would not.
			SameSite: mygo.CookieSameSiteLax,
		}); err != nil {
			l.fail(fmt.Errorf("sign the window in: %w", err))
			return
		}
	}
	l.showURL(origin)
}

func (l *Launcher) showURL(u string) {
	mygo.RunOnMain(func() {
		l.mu.Lock()
		w := l.win
		l.mu.Unlock()
		if w == nil || w.IsDestroyed() {
			return
		}
		if err := w.Page().LoadURL(u); err != nil {
			l.fail(err)
			return
		}
		w.Show()
		w.Focus()
	})
}

func (l *Launcher) showError() {
	mygo.RunOnMain(func() {
		l.mu.Lock()
		w := l.win
		l.mu.Unlock()
		if w == nil || w.IsDestroyed() {
			return
		}
		if err := w.Page().LoadURL("/error.html"); err != nil {
			log.Printf("dsh-go: show the error page: %v", err)
		}
		w.Show()
		w.Focus()
	})
}

func (l *Launcher) fail(err error) {
	l.mu.Lock()
	l.lastErr = err.Error()
	l.mu.Unlock()
	log.Printf("dsh-go: %v", err)
	l.showError()
}

func (l *Launcher) terminate() {
	l.mu.Lock()
	srv := l.srv
	l.mu.Unlock()
	if srv != nil {
		srv.Terminate()
	}
}

func (l *Launcher) shutdown() {
	if l.cancel != nil {
		l.cancel()
	}
	l.mu.Lock()
	srv := l.srv
	l.srv = nil
	logFile := l.logFile
	l.logFile = nil
	l.mu.Unlock()
	if srv != nil {
		_ = srv.Stop(3 * time.Second)
	}
	if logFile != nil {
		_ = logFile.Close()
	}
}

func (l *Launcher) currentURL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil {
		return ""
	}
	return l.srv.URL
}

// Retry shows the splash page and starts the server again.
func (l *Launcher) Retry() {
	l.showSplash()
	go l.start()
}

func (l *Launcher) showSplash() {
	mygo.RunOnMain(func() {
		l.mu.Lock()
		w := l.win
		l.mu.Unlock()
		if w != nil && !w.IsDestroyed() {
			_ = w.Page().LoadURL("/splash.html")
		}
	})
}

// LastError is the reason the last start failed, for the error page.
func (l *Launcher) LastError() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastErr
}

// LogPath is the file dsh writes its output to, empty when it could not be
// opened.
func (l *Launcher) LogPath() string {
	return l.logPath
}

// OpenLogs opens the log directory in the file manager.
func (l *Launcher) OpenLogs() error {
	if l.logPath == "" {
		return errors.New("no log file")
	}
	return mygo.Shell.OpenPath(filepath.Dir(l.logPath))
}

// OpenDataFolder opens the app's data directory.
func (l *Launcher) OpenDataFolder() error {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	return mygo.Shell.OpenPath(dir)
}

// Quit quits the app, as the error page offers.
func (l *Launcher) Quit() { mygo.App.Quit() }

// installedRuntime reports the runtime selected with `dsh-go runtime use`,
// for Discover. A broken selection is logged and left to the bundled tree.
func installedRuntime() (dir, node string) {
	st, err := newRuntimeStore()
	if err != nil {
		log.Printf("dsh-go: installed runtime: %v", err)
		return "", ""
	}
	rt, err := st.Active()
	if err != nil {
		log.Printf("dsh-go: installed runtime: %v", err)
		return "", ""
	}
	if rt == nil {
		return "", ""
	}
	return rt.Dir, rt.NodePath
}

// rollbackRuntime selects the previous runtime and restarts the server.
func (l *Launcher) rollbackRuntime() {
	st, err := newRuntimeStore()
	if err == nil {
		_, err = st.Rollback()
	}
	if err != nil {
		l.menuError("Rollback Runtime", err)
		return
	}
	go l.start()
}

// useBundledRuntime deactivates the selected runtime and restarts the
// server with the bundled one.
func (l *Launcher) useBundledRuntime() {
	st, err := newRuntimeStore()
	if err == nil {
		err = st.Deactivate()
	}
	if err != nil {
		l.menuError("Use Bundled Runtime", err)
		return
	}
	go l.start()
}

func (l *Launcher) openRuntimeFolder() {
	st, err := newRuntimeStore()
	if err == nil {
		err = os.MkdirAll(st.Dir, 0o755)
	}
	if err == nil {
		err = mygo.Shell.OpenPath(st.Dir)
	}
	if err != nil {
		l.menuError("Open Runtime Folder", err)
	}
}

// checkRuntimeUpdates asks the registry whether the channel has a newer
// dsh than the runtime the app would use, and offers to install it.
func (l *Launcher) checkRuntimeUpdates() {
	go func() {
		st, err := newRuntimeStore()
		if err != nil {
			l.menuError("Check Runtime Updates", err)
			return
		}
		channel := updateChannel()
		up, err := st.CheckForUpdate(context.Background(), channel)
		if err != nil {
			l.menuError("Check Runtime Updates", err)
			return
		}
		current, label := currentRuntime(st)
		if !dshruntime.IsNewer(up.Version, current) {
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{
				Type:    mygo.MessageInfo,
				Title:   "Runtime Updates",
				Message: "dsh is up to date.",
				Detail:  fmt.Sprintf("Current: %s (%s)\nChannel %s: %s", orDash(current), label, up.Channel, up.Version),
			})
			return
		}
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Type:    mygo.MessageQuestion,
			Title:   "Runtime Update Available",
			Message: fmt.Sprintf("dsh %s is available.", up.Version),
			Detail:  fmt.Sprintf("Current: %s (%s)\nChannel: %s", orDash(current), label, up.Channel),
			Buttons: []string{"Install", "Later"},
		})
		if err != nil {
			l.menuError("Runtime Update", err)
			return
		}
		if res.Button == 0 {
			l.installRuntime(up.Version)
		}
	}()
}

// installRuntime installs a version and selects it, in the background, then
// offers to restart the server. npm's output goes to the log directory.
func (l *Launcher) installRuntime(version string) {
	l.mu.Lock()
	if l.installing {
		l.mu.Unlock()
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Type:    mygo.MessageInfo,
			Title:   "Runtime Install",
			Message: "An install is already running.",
		})
		return
	}
	l.installing = true
	l.mu.Unlock()

	go func() {
		defer func() {
			l.mu.Lock()
			l.installing = false
			l.mu.Unlock()
		}()
		st, err := newRuntimeStore()
		if err != nil {
			l.menuError("Install Runtime", err)
			return
		}
		logPath := l.installLogPath()
		if logPath != "" {
			if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				defer f.Close()
				st.Log = f
			}
		}
		if mygo.NotificationsSupported() {
			_ = mygo.NewNotification(mygo.NotificationOptions{
				Title: "DSH Go",
				Body:  "Installing dsh " + version + "…",
			}).Show()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		rt, err := st.Install(ctx, version, true)
		if err != nil {
			detail := err.Error()
			if logPath != "" {
				detail += "\n\nLog: " + logPath
			}
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{
				Type:    mygo.MessageError,
				Title:   "Install Runtime",
				Message: fmt.Sprintf("Installing dsh %s failed.", version),
				Detail:  detail,
			})
			return
		}
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Type:    mygo.MessageInfo,
			Title:   "Runtime Installed",
			Message: fmt.Sprintf("dsh %s is installed and selected.", rt.DSH),
			Detail:  "Restart the server now to use it.",
			Buttons: []string{"Restart", "Later"},
		})
		if err == nil && res.Button == 0 {
			go l.start()
		}
	}()
}

func (l *Launcher) installLogPath() string {
	dir := ""
	if l.logPath != "" {
		dir = filepath.Dir(l.logPath)
	}
	if dir == "" {
		if d, err := mygo.App.Path(mygo.PathLogs); err == nil {
			dir = d
		}
	}
	if dir == "" {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return filepath.Join(dir, "runtime-install.log")
}

func (l *Launcher) menuError(title string, err error) {
	log.Printf("dsh-go: %s: %v", title, err)
	if mygo.App.IsReady() {
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Type:    mygo.MessageError,
			Title:   title,
			Message: err.Error(),
		})
	}
}

// viewItems are the View menu's items: the page's history, zoom, the
// workspace in a browser, and full screen.
func (l *Launcher) viewItems() []*mygo.MenuItem {
	view := []*mygo.MenuItem{
		{Role: mygo.RoleReload},
		{Role: mygo.RoleForceReload},
		mygo.Separator(),
		{Role: mygo.RoleResetZoom},
		{Role: mygo.RoleZoomIn},
		{Role: mygo.RoleZoomOut},
		mygo.Separator(),
		{Label: "Open in Browser", Click: func(*mygo.MenuItem, *mygo.Window) {
			if u := l.currentURL(); u != "" {
				if err := mygo.Shell.OpenExternal(u); err != nil {
					log.Printf("dsh-go: open in browser: %v", err)
				}
			}
		}},
		{Role: mygo.RoleToggleFullScreen},
	}
	if mygo.IsDev() {
		view = append(view, mygo.Separator(), &mygo.MenuItem{Role: mygo.RoleToggleDevTools})
	}
	return view
}

// serverItems are the Server menu's items: the dsh server's own lifecycle.
func (l *Launcher) serverItems() []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Label: "Restart", Click: func(*mygo.MenuItem, *mygo.Window) { go l.start() }},
		mygo.Separator(),
		{Label: "Check Runtime Updates…", Click: func(*mygo.MenuItem, *mygo.Window) { l.checkRuntimeUpdates() }},
		{Label: "Rollback Runtime", Click: func(*mygo.MenuItem, *mygo.Window) { l.rollbackRuntime() }},
		{Label: "Use Bundled Runtime", Click: func(*mygo.MenuItem, *mygo.Window) { l.useBundledRuntime() }},
		{Label: "Open Runtime Folder", Click: func(*mygo.MenuItem, *mygo.Window) { l.openRuntimeFolder() }},
		mygo.Separator(),
		{Label: "Open Logs", Click: func(*mygo.MenuItem, *mygo.Window) { _ = l.OpenLogs() }},
		{Label: "Open Data Folder", Click: func(*mygo.MenuItem, *mygo.Window) { _ = l.OpenDataFolder() }},
	}
}

// menu is the window's menu bar, which the keyboard shortcuts reach and a
// window with a page-drawn title bar keeps hidden until Alt or F10.
func (l *Launcher) menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleClose},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "View", Submenu: l.viewItems()},
		{Label: "Server", Submenu: l.serverItems()},
		{Role: mygo.RoleWindowMenu},
	})
}

func webFS() fs.FS {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	return sub
}

// appearance names the appearance the pages and the desktop share.
func appearance() string {
	if mygo.Theme.IsDark() {
		return "dark"
	}
	return "light"
}

func userAgentSuffix() string {
	if v := mygo.App.Version(); v != "" {
		return "DSHGo/" + v
	}
	return "DSHGo"
}

// preloadScript runs in every page before the page's own scripts: the
// bottom corner rounding of internal/titlebar, and the transcript warm-up of
// internal/warmup. DSH_GO_TITLEBAR and DSH_GO_WARMUP turn them off.
func preloadScript() string {
	var parts []string
	if titlebar.Enabled() {
		parts = append(parts, titlebar.Script)
	}
	if warmup.Enabled() {
		parts = append(parts, warmup.Script)
	}
	return strings.Join(parts, "\n;\n")
}
