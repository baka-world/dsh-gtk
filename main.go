// Command dsh-gtk is a MyGo desktop shell for DeepSeek Harness: it starts
// the dsh web server and shows the workspace in a native window.
//
//	go run .
//	go tool mygo build
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/baka-world/dsh-gtk/internal/drawer"
	"github.com/baka-world/dsh-gtk/internal/dsh"
	"github.com/baka-world/dsh-gtk/internal/fps"
	dshruntime "github.com/baka-world/dsh-gtk/internal/runtime"
	"github.com/baka-world/dsh-gtk/internal/titlebar"
	"github.com/baka-world/dsh-gtk/internal/warmup"
	"github.com/egoist/mygo"
)

//go:embed web
var webFiles embed.FS

//go:embed resources/icon.png
var iconPNG []byte

const appName = "DSH GTK"

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
		log.Printf("dsh-gtk: scroll warm-up: on")
	} else {
		log.Printf("dsh-gtk: scroll warm-up: off (DSH_GTK_WARMUP)")
	}
	if titlebar.Enabled() {
		log.Printf("dsh-gtk: title bar: drawn by the page (DSH_GTK_TITLEBAR=0 keeps GTK 3's)")
	} else {
		log.Printf("dsh-gtk: title bar: GTK 3 (DSH_GTK_TITLEBAR)")
	}
	if fps.Enabled() {
		log.Printf("dsh-gtk: frame overlay: on (DSH_GTK_FPS)")
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

	// The window chrome follows the appearance MyGo resolves, which follows
	// the settings portal on Linux; see updateChrome.
	mygo.Theme.OnUpdated(updateChrome)
	mygo.App.OnSecondInstance(func([]string, string) { l.openWindow() })

	mygo.App.WhenReady(func() {
		updateChrome()
		log.Printf("dsh-gtk: appearance: %s", appearance())
		l.openWindow()
		l.addTray()
		go l.start()
		go l.watchRuntimeSelection()
	})

	// A quit starts by closing the windows, and our close handler hides the
	// window instead of closing it whenever a tray can show it again — so the
	// flag has to be up before that, or the quit cancels itself: Quit from the
	// tray, the menu or ⌘Q would do nothing. OnWillQuit comes after the
	// windows close, which is too late for this and the right place for the
	// rest.
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		l.mu.Lock()
		l.quitting = true
		l.mu.Unlock()
	})

	// Quitting stops the server: an agent must not outlive the window that
	// shows it.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		l.mu.Lock()
		l.quitting = true
		l.mu.Unlock()
		l.terminate()
	})
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

// updateChrome gives the window chrome the appearance MyGo resolves: the
// title bar, its buttons and the corners are GTK's, styled in GTK's CSS (see
// internal/titlebar). MyGo follows the desktop's dark style itself now, so
// this only has to restyle on a change, and SetGTKCSS replaces the sheet
// rather than stacking another.
func updateChrome() {
	if !titlebar.Enabled() {
		return
	}
	if err := mygo.Theme.SetGTKCSS(titlebar.GtkCSS(mygo.Theme.IsDark())); err != nil {
		log.Printf("dsh-gtk: window chrome: %v", err)
	}
}

// configureWebRenderer asks MyGo for WebKitGTK's DMA-BUF renderer before the
// toolkit starts: it draws blank windows on NVIDIA, virtual machines and
// containers, so MyGo keeps it off unless the app asks. "auto" leaves it on
// where the host is known to draw with it (Wayland, a Mesa GPU, no container
// or VM), where scrolling a large conversation is 2-5x faster: measured on
// the same 4,586-node chat, 1600x1000, a 12 s continuous scroll ran at 11-19
// fps on the software renderer (WebProcess pinned at 99% of a core) and 56
// fps with DMA-BUF.
//
// DSH_GTK_DMABUF=1 forces it on, 0 forces it off, and anything else, the
// default, asks for "auto". A WEBKIT_DISABLE_DMABUF_RENDERER the caller set
// still wins.
func configureWebRenderer() {
	renderer := rendererFromEnv(os.Getenv("DSH_GTK_DMABUF"))
	mygo.App.SetDMABUFRenderer(renderer)
	log.Printf("dsh-gtk: web renderer: %s (DSH_GTK_DMABUF)", renderer)
}

// rendererFromEnv is the policy of configureWebRenderer, without MyGo.
func rendererFromEnv(value string) mygo.DMABUFRenderer {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "on", "true", "dma", "dmabuf":
		return mygo.DMABUFRendererOn
	case "0", "off", "false", "software":
		return mygo.DMABUFRendererOff
	}
	return mygo.DMABUFRendererAuto
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.workspace, "workspace", os.Getenv("DSH_GTK_WORKSPACE"), "directory dsh works in (default: your home directory)")
	flag.StringVar(&cfg.home, "home", os.Getenv("DSH_GTK_HOME"), "DSH_HOME of the server (default: dsh's own, ~/.dsh)")
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
	tray       *mygo.Tray
	quitting   bool

	// runningID and runningDSH are the dsh tree the server started from;
	// managed is false when it came from DSH_GTK_DSH or PATH, which the
	// runtime store does not select. The watcher compares them with the
	// selection, and promptedSelection is what it has already offered.
	runningID         string
	runningDSH        string
	managed           bool
	promptedSelection string
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

	w.OnClose(func(e *mygo.CloseEvent) {
		// With a tray the window hides instead of quitting: dsh and the task
		// it runs keep going in the background, and the tray brings the
		// window back. Quit from the tray or the menu really quits.
		l.mu.Lock()
		keep := l.tray != nil && !l.quitting
		l.mu.Unlock()
		if keep {
			e.PreventDefault()
			w.Hide()
		}
	})
	w.OnClosed(func() {
		l.mu.Lock()
		if l.win == w {
			l.win = nil
		}
		l.mu.Unlock()
	})
	if fps.Enabled() && fps.LogEnabled() {
		go l.logFPS(w)
	}
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

	// The shell binds the port and hands the socket to dsh (see
	// internal/dsh/listenfd.js): dsh cannot fail to bind it, there is no window
	// between picking a port and dsh using it, and the port stays the same on
	// every start — the web UI's localStorage, its sidebar and selected session
	// included, is keyed by origin, and the origin contains the port.
	port := l.cfg.port
	var listen *os.File
	if port != 0 {
		ln, bound := bindPort(port, portTries)
		if ln == nil {
			log.Printf("dsh-gtk: ports %d-%d are taken; asking the OS for one", port, port+portTries-1)
			port = 0
		} else {
			port = bound
			if f, err := ln.(*net.TCPListener).File(); err == nil {
				listen = f
			} else {
				log.Printf("dsh-gtk: listening socket: %v", err)
			}
			_ = ln.Close()
		}
	}
	srv, err := l.boot(port, listen)
	if listen != nil {
		_ = listen.Close() // the child holds its own copy
	}
	if err != nil && port != 0 && errors.Is(err, dsh.ErrAddrInUse) {
		// dsh bound the port itself instead of adopting the socket, so it was
		// still ours: start it again on a port the OS picks.
		log.Printf("dsh-gtk: port %d is taken; asking for a free one", port)
		srv, err = l.boot(0, nil)
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
		log.Printf("dsh-gtk: %v", err)
		l.showError()
		return
	}
	l.srv = srv
	l.mu.Unlock()

	l.showWorkspace(srv, gen)
	go l.watch(srv, gen)
}

// portTries is how many ports from the preferred one are probed before giving
// up and letting the server pick any.
const portTries = 10

// bindPort takes the first port from start upward that can be listened on and
// returns its listener, or nil when none of them can. The caller keeps the
// socket and hands it to dsh.
func bindPort(start, tries int) (net.Listener, int) {
	for p := start; p < start+tries; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err != nil {
			continue
		}
		return ln, p
	}
	return nil, 0
}

func (l *Launcher) boot(port int, listen *os.File) (*dsh.Server, error) {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return nil, fmt.Errorf("find the app resources: %w", err)
	}
	installed := installedRuntimeInfo()
	installedDir, installedNode := "", ""
	if installed != nil {
		installedDir, installedNode = installed.Dir, installed.NodePath
	}
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
	log.Printf("dsh-gtk: %s runtime: %s", launch.Source, launch.Executable)
	l.noteRunning(resources, launch, installed)
	if drawer.Enabled() {
		home := l.cfg.home
		if home == "" {
			home, _ = os.UserHomeDir()
			home = filepath.Join(home, ".dsh")
		}
		if err := drawer.Install(home); err != nil {
			log.Printf("dsh-gtk: drawer plugin: %v", err)
		}
	}
	var w io.Writer = os.Stderr
	if l.logFile != nil {
		w = l.logFile
	}
	shim := ""
	if listen != nil {
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
			if p, err := dsh.WriteShim(dir); err == nil {
				shim = p
			} else {
				log.Printf("dsh-gtk: listen shim: %v", err)
			}
		}
	}
	if shim == "" {
		// Without the preload dsh would try to bind the socket we hold and fail.
		listen = nil
	}
	return dsh.Start(dsh.Config{
		Launch:       launch,
		Workspace:    l.cfg.workspace,
		Home:         l.cfg.home,
		Port:         port,
		Listen:       listen,
		ListenShim:   shim,
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

// addTray puts the app in the notification area, where a desktop keeps a
// running app reachable after its window is closed to it: the icon shows and
// hides the window, and quits. A desktop without a StatusNotifier host (or a
// system without libappindicator) has no tray, which is not an error.
func (l *Launcher) addTray() {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Show " + appName, Click: func(*mygo.MenuItem, *mygo.Window) { l.openWindow() }},
		mygo.Separator(),
		{Label: "Quit", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "main",
		Icon:    iconPNG,
		Title:   appName,
		ToolTip: appName,
		Menu:    menu,
	})
	if err != nil {
		// Unsupported on a desktop without a tray, or a system without the
		// appindicator library: the window and its menu are enough there.
		log.Printf("dsh-gtk: no tray icon: %v", err)
		return
	}
	l.mu.Lock()
	l.tray = tray
	l.mu.Unlock()
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
			log.Printf("dsh-gtk: show the error page: %v", err)
		}
		w.Show()
		w.Focus()
	})
}

func (l *Launcher) fail(err error) {
	l.mu.Lock()
	l.lastErr = err.Error()
	l.mu.Unlock()
	log.Printf("dsh-gtk: %v", err)
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

// installedRuntimeInfo reports the runtime selected with `dsh-gtk runtime use`,
// or nil when the bundled tree is selected. A broken selection is logged and
// left to the bundled tree.
func installedRuntimeInfo() *dshruntime.Runtime {
	rt, err := activeSelection()
	if err != nil {
		log.Printf("dsh-gtk: installed runtime: %v", err)
		return nil
	}
	return rt
}

// activeSelection reads the selection without logging, for callers that poll
// it and report failure their own way.
func activeSelection() (*dshruntime.Runtime, error) {
	st, err := newRuntimeStore()
	if err != nil {
		return nil, err
	}
	return st.Active()
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
// dsh than the runtime the app would use, and offers to install it. With no
// channel set the check follows the tag of that runtime, so an installed
// prerelease keeps seeing its own next release.
func (l *Launcher) checkRuntimeUpdates() {
	go func() {
		st, err := newRuntimeStore()
		if err != nil {
			l.menuError("Check Runtime Updates", err)
			return
		}
		current, label := currentRuntime(st)
		channel := updateChannel()
		up, err := st.CheckForUpdate(context.Background(), channel, current)
		if err != nil {
			l.menuError("Check Runtime Updates", err)
			return
		}
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
				Title: "DSH GTK",
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
	log.Printf("dsh-gtk: %s: %v", title, err)
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
					log.Printf("dsh-gtk: open in browser: %v", err)
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
		return "DSHGTK/" + v
	}
	return "DSHGTK"
}

// preloadScript runs in every page before the page's own scripts: the
// bottom corner rounding of internal/titlebar, and the transcript warm-up of
// internal/warmup. DSH_GTK_TITLEBAR and DSH_GTK_WARMUP turn them off.
func preloadScript() string {
	var parts []string
	if titlebar.Enabled() {
		parts = append(parts, titlebar.Script)
	}
	if warmup.Enabled() {
		parts = append(parts, warmup.Script)
	}
	if fps.Enabled() {
		parts = append(parts, fps.Script)
	}
	if css := userCSS(); css != "" {
		parts = append(parts, css)
	}
	return strings.Join(parts, "\n;\n")
}

// userCSS returns a script that appends the stylesheet named by DSH_GTK_CSS to
// every page, so an experiment on the page's layout costs a restart instead of
// a rebuild. The file is read when the window is created: change it, then
// reload the page (View → Reload) or restart the app.
func userCSS() string {
	path := strings.TrimSpace(os.Getenv("DSH_GTK_CSS"))
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("dsh-gtk: DSH_GTK_CSS: %v", err)
		return ""
	}
	log.Printf("dsh-gtk: page CSS: %d bytes from %s", len(data), path)
	quoted, err := json.Marshal(string(data))
	if err != nil {
		return ""
	}
	return `(function () {
  if (window.__dshUserCSS) return;
  window.__dshUserCSS = true;
  var style = document.createElement('style');
  style.setAttribute('data-dsh-user-css', '');
  style.textContent = ` + string(quoted) + `;
  (document.head || document.documentElement).appendChild(style);
})();`
}
