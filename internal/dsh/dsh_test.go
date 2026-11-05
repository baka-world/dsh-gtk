package dsh

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseReady(t *testing.T) {
	tests := []struct {
		line string
		want string
		ok   bool
	}{
		{"dsh web: http://127.0.0.1:3080/?token=abc", "http://127.0.0.1:3080/?token=abc", true},
		{"dsh web: https://localhost:8443/ui/?token=abc", "https://localhost:8443/ui/?token=abc", true},
		{"dsh web: http://127.0.0.1:3080/", "http://127.0.0.1:3080/", true},
		{"dsh web: http://[::1]:3080/?token=abc", "http://[::1]:3080/?token=abc", true},
		{"dsh web: not a url", "", false},
		{"dsh web: ftp://127.0.0.1:3080/", "", false},
		{"dsh web: http:///no-host", "", false},
		{"other: http://127.0.0.1:3080/", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := parseReady(tt.line)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseReady(%q) = %q, %v; want %q, %v", tt.line, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDiscoverPrefersBundle(t *testing.T) {
	res := t.TempDir()
	node := filepath.Join(res, "runtime", "node")
	bin := filepath.Join(res, "runtime", "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
	writeFile(t, node, "#!/bin/sh\n", 0o755)
	writeFile(t, bin, "", 0o644)

	got, err := Discover(DiscoverOptions{
		Resources: res,
		Getenv:    func(string) string { return "" },
		LookPath:  func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "bundle" || got.Executable != node {
		t.Errorf("Launch = %+v; want bundled node", got)
	}
	if len(got.Args) != 1 || got.Args[0] != bin {
		t.Errorf("Args = %q; want the bundled bin.js", got.Args)
	}
	if got.NodeDir != filepath.Join(res, "runtime") {
		t.Errorf("NodeDir = %q", got.NodeDir)
	}
}

func TestDiscoverBundleFallsBackToPATHNode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix node name")
	}
	res := t.TempDir()
	writeFile(t, filepath.Join(res, "runtime", "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js"), "", 0o644)

	got, err := Discover(DiscoverOptions{
		Resources: res,
		Getenv:    func(string) string { return "" },
		LookPath:  func(name string) (string, error) { return "/usr/bin/" + name, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Executable != "/usr/bin/node" || got.Source != "bundle" {
		t.Errorf("Launch = %+v; want node from PATH", got)
	}
}

func TestDiscoverExplicitRuntime(t *testing.T) {
	res := t.TempDir()
	dir := t.TempDir()
	script := filepath.Join(dir, "bin.js")
	writeFile(t, script, "", 0o644)
	node := filepath.Join(dir, "node")
	writeFile(t, node, "#!/bin/sh\n", 0o755)

	got, err := Discover(DiscoverOptions{
		Resources: res,
		DSH:       script,
		Node:      node,
		Getenv:    func(string) string { return "" },
		LookPath:  func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Executable != node || len(got.Args) != 1 || got.Args[0] != script {
		t.Errorf("Launch = %+v; want node %s %s", got, node, script)
	}
}

func TestDiscoverPrefersInstalledRuntime(t *testing.T) {
	res := t.TempDir()
	installed := t.TempDir()
	bin := filepath.Join(installed, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
	writeFile(t, bin, "", 0o644)

	got, err := Discover(DiscoverOptions{
		Resources:    res,
		InstalledDir: installed,
		Getenv:       func(string) string { return "" },
		LookPath:     func(string) (string, error) { return "/usr/bin/node", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "installed" || got.Executable != "/usr/bin/node" {
		t.Errorf("Launch = %+v; want the installed tree on the PATH node", got)
	}
	if len(got.Args) != 1 || got.Args[0] != bin {
		t.Errorf("Args = %q; want the installed bin.js", got.Args)
	}
}

func TestDiscoverUsesInstalledNode(t *testing.T) {
	res := t.TempDir()
	installed := t.TempDir()
	node := filepath.Join(installed, "node")
	writeFile(t, node, "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(installed, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js"), "", 0o644)

	got, err := Discover(DiscoverOptions{
		Resources:     res,
		InstalledDir:  installed,
		InstalledNode: node,
		Getenv:        func(string) string { return "" },
		LookPath:      func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "installed" || got.Executable != node || got.NodeDir != installed {
		t.Errorf("Launch = %+v; want the runtime's own node", got)
	}
}

func TestDiscoverIgnoresBrokenInstalledRuntime(t *testing.T) {
	res := t.TempDir()
	writeFile(t, filepath.Join(res, "runtime", "node"), "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(res, "runtime", "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js"), "", 0o644)

	got, err := Discover(DiscoverOptions{
		Resources:    res,
		InstalledDir: t.TempDir(), // no dsh inside
		Getenv:       func(string) string { return "" },
		LookPath:     func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "bundle" {
		t.Errorf("Launch = %+v; want the bundle", got)
	}
}

func TestDiscoverFallsBackToPATH(t *testing.T) {
	got, err := Discover(DiscoverOptions{
		Resources: t.TempDir(),
		Getenv:    func(string) string { return "" },
		LookPath:  func(name string) (string, error) { return "/usr/local/bin/" + name, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Executable != "/usr/local/bin/dsh" || got.Source != "PATH" || len(got.Args) != 0 {
		t.Errorf("Launch = %+v; want dsh from PATH", got)
	}
}

func TestDiscoverReportsMissingRuntime(t *testing.T) {
	_, err := Discover(DiscoverOptions{
		Resources: t.TempDir(),
		Getenv:    func(string) string { return "" },
		LookPath:  func(string) (string, error) { return "", errors.New("not found") },
	})
	if err == nil || !strings.Contains(err.Error(), "no dsh runtime found") {
		t.Fatalf("err = %v; want a missing runtime error", err)
	}
}

func TestServerReadsURLAndStops(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	srv := startFake(t, `#!/bin/sh
trap 'exit 0' TERM
echo 'dsh web: http://127.0.0.1:4567/?token=abc'
while :; do sleep 0.05; done
`, 10*time.Second)

	if srv.URL != "http://127.0.0.1:4567/?token=abc" || srv.Port != 4567 {
		t.Fatalf("URL = %q, Port = %d", srv.URL, srv.Port)
	}
	if err := srv.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !srv.Stopping() {
		t.Error("Stopping() = false after Stop")
	}
}

func TestServerReportsAddressInUse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	_, err := startScript(t, `#!/bin/sh
echo 'Error: listen EADDRINUSE: address already in use 127.0.0.1:3080' >&2
exit 1
`, 5*time.Second)
	if !errors.Is(err, ErrAddrInUse) {
		t.Fatalf("err = %v; want ErrAddrInUse", err)
	}
	if !strings.Contains(err.Error(), "EADDRINUSE") {
		t.Errorf("err = %q; want dsh's output", err)
	}
}

func TestServerReportsEarlyExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	_, err := startScript(t, `#!/bin/sh
echo 'plugin failed to load' >&2
exit 3
`, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "plugin failed to load") {
		t.Fatalf("err = %v; want the child's output", err)
	}
	if errors.Is(err, ErrAddrInUse) {
		t.Error("ErrAddrInUse for an unrelated failure")
	}
}

func startFake(t *testing.T, script string, timeout time.Duration) *Server {
	t.Helper()
	srv, err := startScript(t, script, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func startScript(t *testing.T, script string, timeout time.Duration) (*Server, error) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "dsh")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		return nil, err
	}
	return Start(Config{
		Launch:       Launch{Executable: bin, Source: "test"},
		Workspace:    dir,
		Port:         0,
		ReadyTimeout: timeout,
	})
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestOriginDropsToken(t *testing.T) {
	s := &Server{URL: "http://127.0.0.1:3080/?token=abc"}
	if got := s.Origin(); got != "http://127.0.0.1:3080/" {
		t.Fatalf("Origin() = %q", got)
	}
}

func TestAuthCookiesExchangesToken(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("token") != "abc" {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "dsh-auth-x", Value: "v1.sig", Path: "/", MaxAge: 60})
		http.Redirect(w, r, "./", http.StatusSeeOther)
	}))
	defer srv.Close()

	s := &Server{URL: srv.URL + "/?token=abc"}
	cookies, err := s.AuthCookies(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Errorf("the client made %d requests; want 1", requests)
	}
	if len(cookies) != 1 || cookies[0].Name != "dsh-auth-x" || cookies[0].Value != "v1.sig" {
		t.Fatalf("cookies = %+v", cookies)
	}
}

func TestAuthCookiesReportsRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "authentication required", http.StatusUnauthorized)
	}))
	defer srv.Close()

	s := &Server{URL: srv.URL + "/?token=bad"}
	if _, err := s.AuthCookies(5 * time.Second); err == nil {
		t.Fatal("AuthCookies accepted a refusal")
	}
}

func TestStartStopsWithContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := `#!/bin/sh
echo $$ > ` + pidFile + `
while :; do sleep 0.1; done
`
	bin := filepath.Join(dir, "dsh")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		_, err := Start(Config{
			Launch:       Launch{Executable: bin, Source: "test"},
			Workspace:    dir,
			ReadyTimeout: 30 * time.Second,
			Ctx:          ctx,
		})
		errc <- err
	}()

	var pid int
	deadline := time.Now().Add(10 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		if b, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("the fake dsh never started")
	}

	cancel()
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Start = %v; want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Start did not return after cancel")
	}

	for i := 0; i < 100; i++ {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child %d still runs after the context was canceled", pid)
}

// TestServerRedactsLaunchToken keeps the one-time token out of the log file
// and the diagnostics tail: a log outlives the server that issued it.
func TestServerRedactsLaunchToken(t *testing.T) {
	var buf bytes.Buffer
	s := &Server{log: &buf}
	s.record("dsh web: http://127.0.0.1:3080/?token=s3cret-Token_1")
	s.record("plain line")
	logged := buf.String()
	if strings.Contains(logged, "s3cret") {
		t.Errorf("log kept the token: %q", logged)
	}
	if !strings.Contains(logged, "token=…") {
		t.Errorf("log does not mark the redaction: %q", logged)
	}
	if !strings.Contains(logged, "plain line") {
		t.Errorf("log dropped a line: %q", logged)
	}
	if tail := s.Tail(); len(tail) == 0 || strings.Contains(strings.Join(tail, "\n"), "s3cret") {
		t.Errorf("tail kept the token: %q", tail)
	}
}

// TestServerPassesTheListeningSocket pins the wiring that keeps the web UI's
// origin stable: the shell's socket reaches the child as fd 3, and node is told
// to preload the script that adopts it.
func TestServerPassesTheListeningSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script and /proc")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	f, err := ln.(*net.TCPListener).File()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = ln.Close()

	dir := t.TempDir()
	node := filepath.Join(dir, "node") // named node: the preload is for node only
	script := `#!/bin/sh
if [ -e /proc/self/fd/3 ]; then echo "fd3=open" >&2; else echo "fd3=closed" >&2; fi
echo "fd=$DSH_GTK_LISTEN_FD port=$DSH_GTK_LISTEN_PORT args=$*" >&2
echo 'dsh web: http://127.0.0.1:4567/?token=abc'
sleep 1
`
	if err := os.WriteFile(node, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	srv, err := Start(Config{
		Launch:       Launch{Executable: node, Args: []string{"bin.js"}},
		Workspace:    dir,
		Port:         port,
		Listen:       f,
		ListenShim:   filepath.Join(dir, "listenfd.js"),
		Log:          &log,
		ReadyTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop(time.Second)

	out := log.String()
	for _, want := range []string{
		"fd3=open",
		"fd=3",
		"port=" + strconv.Itoa(port),
		"--require " + filepath.Join(dir, "listenfd.js"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("child saw %q, not %q", out, want)
		}
	}
}

// TestServerBindsItselfWithoutASocket keeps the old path: no Listen means dsh
// binds the port and the preload is not added.
func TestServerBindsItselfWithoutASocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	dir := t.TempDir()
	node := filepath.Join(dir, "node")
	script := `#!/bin/sh
echo "args=$* fd=${DSH_GTK_LISTEN_FD:-none}" >&2
echo 'dsh web: http://127.0.0.1:4567/?token=abc'
sleep 1
`
	if err := os.WriteFile(node, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	srv, err := Start(Config{
		Launch:       Launch{Executable: node, Args: []string{"bin.js"}},
		Workspace:    dir,
		Port:         3080,
		Log:          &log,
		ReadyTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop(time.Second)
	out := log.String()
	if strings.Contains(out, "--require") || strings.Contains(out, "fd=3") {
		t.Errorf("child got the preload without a socket: %q", out)
	}
}
