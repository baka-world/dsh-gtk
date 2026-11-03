package dsh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// readyPrefix is what `dsh web` prints once the server accepts requests.
const readyPrefix = "dsh web: "

const (
	defaultReadyTimeout = 2 * time.Minute
	defaultStopTimeout  = 5 * time.Second
	maxTailLines        = 80
)

// ErrAddrInUse is wrapped by Start when dsh exited because its port was
// taken, so the caller can retry on a free one.
var ErrAddrInUse = errors.New("dsh: address already in use")

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Config starts a dsh web server.
type Config struct {
	// Launch is the runtime Discover found.
	Launch Launch
	// Workspace is the directory dsh works in; empty means the user's home.
	Workspace string
	// Home is DSH_HOME for the child; empty leaves the environment alone
	// and dsh uses ~/.dsh.
	Home string
	// Port is the listening port; 0 asks the OS for a free one.
	Port int
	// Log receives every line dsh writes, with a timestamp. nil discards.
	Log io.Writer
	// ReadyTimeout bounds the wait for the URL line (2 minutes).
	ReadyTimeout time.Duration
	// Ctx ends the server when the app quits before or during boot. nil
	// means no cancellation.
	Ctx context.Context
}

// Server is a running dsh web server.
type Server struct {
	// URL is the authenticated URL dsh printed: it sets the auth cookie
	// and redirects to the app root.
	URL string
	// Port is the port the server listens on.
	Port int

	cmd *exec.Cmd
	log io.Writer

	mu      sync.Mutex
	tail    []string
	writeMu sync.Mutex
	logErr  error

	stopping atomic.Bool
	done     chan struct{}
	exitErr  error
}

// Start runs `dsh web --no-open` and waits for the URL line.
func Start(cfg Config) (*Server, error) {
	if cfg.Launch.Executable == "" {
		return nil, errors.New("dsh: no executable")
	}
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = defaultReadyTimeout
	}
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}
	if cfg.Workspace == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("dsh: find a workspace: %w", err)
		}
		cfg.Workspace = home
	}
	if st, err := os.Stat(cfg.Workspace); err != nil {
		return nil, fmt.Errorf("dsh: workspace %s: %w", cfg.Workspace, err)
	} else if !st.IsDir() {
		return nil, fmt.Errorf("dsh: workspace %s is not a directory", cfg.Workspace)
	}

	args := []string{
		"web",
		"--no-open",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(cfg.Port),
	}
	prog, argv := cfg.Launch.Command(args...)

	cmd := exec.Command(prog, argv...)
	cmd.Dir = cfg.Workspace
	cmd.Env = childEnv(cfg)
	configureProcess(cmd)

	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	cmd.Stdout = outW
	cmd.Stderr = errW
	if err := cmd.Start(); err != nil {
		outR.Close()
		outW.Close()
		errR.Close()
		errW.Close()
		return nil, fmt.Errorf("dsh: start %s: %w", prog, err)
	}

	s := &Server{cmd: cmd, log: cfg.Log, done: make(chan struct{})}
	ready := make(chan string, 1)
	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		s.scan(outR, ready)
	}()
	go func() {
		defer readers.Done()
		s.scan(errR, nil)
	}()
	go func() {
		err := cmd.Wait()
		outW.Close()
		errW.Close()
		readers.Wait()
		s.exitErr = err
		close(s.done)
	}()

	ctx := cfg.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(cfg.ReadyTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		s.Terminate()
		select {
		case <-s.done:
		case <-time.After(defaultStopTimeout):
			s.Kill()
			<-s.done
		}
		return nil, fmt.Errorf("dsh: %w", ctx.Err())
	case raw := <-ready:
		u, err := url.Parse(raw)
		if err != nil || u.Port() == "" {
			_ = s.Stop(defaultStopTimeout)
			return nil, fmt.Errorf("dsh: unreadable URL %q", raw)
		}
		port, _ := strconv.Atoi(u.Port())
		s.URL = raw
		s.Port = port
		return s, nil
	case <-s.done:
		return nil, s.failure("dsh exited before it was ready")
	case <-timer.C:
		s.Terminate()
		select {
		case <-s.done:
		case <-time.After(defaultStopTimeout):
			s.Kill()
			<-s.done
		}
		return nil, s.failure(fmt.Sprintf("dsh did not report its URL within %s", cfg.ReadyTimeout))
	}
}

// Done is closed when the server process exits and its output is read.
func (s *Server) Done() <-chan struct{} { return s.done }

// Origin is the server's root URL, without the launch token.
func (s *Server) Origin() string {
	u, err := url.Parse(s.URL)
	if err != nil {
		return s.URL
	}
	u.Path = "/"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// AuthCookies exchanges the authenticated URL for the cookies that sign a
// page in. The server answers the token URL with a redirect and a cookie;
// the redirect is not followed.
//
// A webview cannot rely on that exchange: the navigation starts on the
// app's own scheme, so a SameSite=Strict cookie the redirect sets is
// withheld from the redirected request, and the page lands on the
// server's unauthenticated answer. Fetching it here lets the shell put
// the cookie in the webview's session first.
func (s *Server) AuthCookies(timeout time.Duration) ([]*http.Cookie, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(s.URL)
	if err != nil {
		return nil, fmt.Errorf("dsh: exchange the launch token: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return nil, fmt.Errorf("dsh: the launch URL answered %s without a cookie", resp.Status)
	}
	return cookies, nil
}

// Err returns why the server exited, or nil when it exited cleanly or is
// still running.
func (s *Server) Err() error {
	select {
	case <-s.done:
		return s.exitErr
	default:
		return nil
	}
}

// Stopping reports whether Stop or Terminate asked the server to exit.
func (s *Server) Stopping() bool { return s.stopping.Load() }

// Tail returns the last lines dsh wrote, newest last.
func (s *Server) Tail() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.tail...)
}

// AddrInUse reports whether dsh failed because the port was taken.
func (s *Server) AddrInUse() bool {
	for _, line := range s.Tail() {
		if strings.Contains(line, "EADDRINUSE") {
			return true
		}
	}
	return false
}

// Terminate asks the server to exit, without waiting. It is safe to call
// more than once, and after the server exited.
func (s *Server) Terminate() {
	s.stopping.Store(true)
	terminateProcess(s.cmd)
}

// Kill ends the server at once.
func (s *Server) Kill() {
	s.stopping.Store(true)
	killProcess(s.cmd)
}

// Stop asks the server to exit and waits for it, killing it after timeout.
func (s *Server) Stop(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultStopTimeout
	}
	select {
	case <-s.done:
		return nil
	default:
	}
	s.stopping.Store(true)
	terminateProcess(s.cmd)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.done:
		return nil
	case <-timer.C:
		killProcess(s.cmd)
		<-s.done
		return fmt.Errorf("dsh: killed after it did not exit within %s", timeout)
	}
}

// scan copies a stream of the child to the log and looks for the URL line.
func (s *Server) scan(r io.Reader, ready chan<- string) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			line = ansiEscape.ReplaceAllString(strings.TrimRight(line, "\r\n"), "")
			s.record(line)
			if ready != nil {
				if u, ok := parseReady(line); ok {
					select {
					case ready <- u:
					default:
					}
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *Server) record(line string) {
	s.mu.Lock()
	s.tail = append(s.tail, line)
	if len(s.tail) > maxTailLines {
		s.tail = append([]string(nil), s.tail[len(s.tail)-maxTailLines:]...)
	}
	s.mu.Unlock()

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.logErr != nil {
		return
	}
	if _, err := fmt.Fprintf(s.log, "%s dsh | %s\n", time.Now().Format("2006-01-02T15:04:05.000"), line); err != nil {
		s.logErr = err
	}
}

func (s *Server) failure(msg string) error {
	detail := ""
	if err := s.Err(); err != nil {
		detail = "\n" + err.Error()
	}
	if tail := s.Tail(); len(tail) > 0 {
		detail += "\n" + strings.Join(tail, "\n")
	}
	if s.AddrInUse() {
		return fmt.Errorf("%w: %s%s", ErrAddrInUse, msg, detail)
	}
	return errors.New(msg + detail)
}

// parseReady reads the URL out of the line `dsh web` prints when ready.
func parseReady(line string) (string, bool) {
	if !strings.HasPrefix(line, readyPrefix) {
		return "", false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(line, readyPrefix))
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return raw, true
}

// childEnv is the environment of the dsh process: the app's, with DSH_HOME,
// the bundled node first on PATH, and colors off so the URL line stays
// plain.
func childEnv(cfg Config) []string {
	env := os.Environ()
	if cfg.Home != "" {
		env = setEnv(env, "DSH_HOME", cfg.Home)
	}
	if cfg.Launch.NodeDir != "" {
		env = setEnv(env, "PATH", cfg.Launch.NodeDir+string(filepath.ListSeparator)+getEnv(env, "PATH"))
	}
	env = setEnv(env, "NO_COLOR", "1")
	env = setEnv(env, "FORCE_COLOR", "0")
	return env
}

func setEnv(env []string, key, value string) []string {
	for i, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			env[i] = key + "=" + value
			return env
		}
	}
	return append(env, key+"="+value)
}

func getEnv(env []string, key string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
