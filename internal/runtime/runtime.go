// Package runtime installs and selects DeepSeek Harness runtimes that live
// outside the app binary.
//
// A runtime is the JS tree of one dsh version: node_modules/@deepseek-ai/dsh
// and its dependencies, installed with npm. Node itself stays with the app.
// Installed runtimes live in <userData>/runtimes/<id>, the selection in
// <userData>/runtime.json, and the bundled runtime in the app's resources.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Manifest is the runtime.json file of a runtime directory.
type Manifest struct {
	// DSH is the version of @deepseek-ai/dsh.
	DSH string `json:"dsh"`
	// Node is the Node version the runtime was verified with, when known.
	Node string `json:"node,omitempty"`
	// Source says how it arrived: "npm", "bundle".
	Source string `json:"source,omitempty"`
	// InstalledAt is an RFC 3339 time.
	InstalledAt string `json:"installedAt,omitempty"`
}

// Runtime is one dsh tree on disk.
type Runtime struct {
	// ID is the directory name under the store, e.g. "dsh-0.2.2-alpha.3".
	ID string `json:"id"`
	// Dir is the tree, holding node_modules/@deepseek-ai/dsh.
	Dir string `json:"dir"`
	// DSH is the dsh version.
	DSH string `json:"dsh"`
	// NodeVersion is the Node version the runtime was verified with.
	NodeVersion string `json:"nodeVersion,omitempty"`
	// NodePath is a node executable inside Dir, when the runtime ships one.
	NodePath string `json:"nodePath,omitempty"`
	// Bundled tells that this runtime came with the app.
	Bundled bool `json:"bundled,omitempty"`
}

const (
	manifestName = "runtime.json"
	stateName    = "runtime.json"
)

// Store keeps installed runtimes and the selection between them.
type Store struct {
	// Dir is where installed runtimes go, e.g. <userData>/runtimes.
	Dir string
	// State is the selection file, e.g. <userData>/runtime.json.
	State string
	// Node runs npm and the installed runtime.
	Node string
	// NPM is npm's CLI script, run as `node NPM ...`.
	NPM string
	// Log receives npm's output. nil discards.
	Log io.Writer
}

// NewStore returns a store of runtimes under userData. node and npm are
// needed to install, not to read the selection.
func NewStore(userData, node, npm string) *Store {
	return &Store{
		Dir:   filepath.Join(userData, "runtimes"),
		State: filepath.Join(userData, stateName),
		Node:  node,
		NPM:   npm,
	}
}

// selection is the state file.
type selection struct {
	Active   string `json:"active,omitempty"`
	Previous string `json:"previous,omitempty"`
}

// Active returns the selected installed runtime, or nil when none is.
func (s *Store) Active() (*Runtime, error) {
	sel, err := s.readSelection()
	if err != nil {
		return nil, err
	}
	if sel.Active == "" {
		return nil, nil
	}
	rt, err := s.open(sel.Active)
	if err != nil {
		return nil, fmt.Errorf("active runtime %q: %w", sel.Active, err)
	}
	return rt, nil
}

// Previous returns the runtime that Rollback would select, or nil.
func (s *Store) Previous() (*Runtime, error) {
	sel, err := s.readSelection()
	if err != nil {
		return nil, err
	}
	if sel.Previous == "" {
		return nil, nil
	}
	rt, err := s.open(sel.Previous)
	if err != nil {
		return nil, fmt.Errorf("previous runtime %q: %w", sel.Previous, err)
	}
	return rt, nil
}

// List returns the installed runtimes, oldest first by modification time.
func (s *Store) List() ([]Runtime, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Runtime
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		rt, err := s.open(e.Name())
		if err != nil {
			continue
		}
		out = append(out, *rt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DSH < out[j].DSH })
	return out, nil
}

// open reads a runtime directory, checking that dsh is there.
func (s *Store) open(id string) (*Runtime, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return nil, fmt.Errorf("invalid runtime id %q", id)
	}
	dir := filepath.Join(s.Dir, id)
	bin := filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("no dsh in %s", dir)
	}
	rt := &Runtime{ID: id, Dir: dir}
	m, err := readManifest(dir)
	if err != nil {
		m, err = manifestFromPackage(dir)
		if err != nil {
			return nil, err
		}
	}
	rt.DSH = m.DSH
	rt.NodeVersion = m.Node
	for _, name := range []string{"node", "node.exe"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && (st.Mode().Perm()&0o111 != 0 || name == "node.exe") {
			rt.NodePath = p
			break
		}
	}
	return rt, nil
}

// Bundled describes the runtime that ships with the app, in
// <resources>/runtime.
func Bundled(resources string) (*Runtime, error) {
	dir := filepath.Join(resources, "runtime")
	bin := filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("no bundled runtime in %s", dir)
	}
	rt := &Runtime{ID: "bundled", Dir: dir, Bundled: true}
	m, err := readManifest(dir)
	if err != nil {
		m, err = manifestFromPackage(dir)
		if err != nil {
			return nil, err
		}
	}
	rt.DSH = m.DSH
	rt.NodeVersion = m.Node
	for _, name := range []string{"node", "node.exe"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			rt.NodePath = p
			break
		}
	}
	return rt, nil
}

// Install fetches @deepseek-ai/dsh@spec with npm into a new runtime and, when
// activate is true, selects it. spec is a version or a dist-tag such as
// "latest". An installed runtime that fails verification is removed.
func (s *Store) Install(ctx context.Context, spec string, activate bool) (*Runtime, error) {
	if s.Node == "" || s.NPM == "" {
		return nil, errors.New("installing a runtime needs node and npm; run scripts/prepare-runtime.sh or install npm")
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, errors.New("no dsh version")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(s.Dir, ".staging-")
	if err != nil {
		return nil, err
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.RemoveAll(staging)
		}
	}()

	pkgSpec := "@deepseek-ai/dsh@" + spec
	nodeVersion, err := nodeVersionOf(s.Node)
	if err != nil {
		return nil, err
	}
	args := []string{s.NPM, "install", "--prefix", staging, "--omit=dev", "--no-audit", "--no-fund",
		"--loglevel=error", pkgSpec}
	cmd := exec.CommandContext(ctx, s.Node, args...)
	cmd.Dir = staging
	cmd.Env = append(os.Environ(),
		"npm_config_cache="+filepath.Join(filepath.Dir(s.Dir), "npm-cache"),
		"npm_config_update_notifier=false",
		"npm_config_fund=false",
		"npm_config_progress=false",
	)
	tail := &tailWriter{max: 30}
	var out io.Writer = tail
	if s.Log != nil {
		out = io.MultiWriter(s.Log, tail)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("npm install %s: %w\n%s", pkgSpec, err, strings.Join(tail.lines, "\n"))
	}

	pkgDir := filepath.Join(staging, "node_modules", "@deepseek-ai", "dsh")
	var pkg struct {
		Version string `json:"version"`
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if err := readJSON(filepath.Join(pkgDir, "package.json"), &pkg); err != nil {
		return nil, fmt.Errorf("read the installed package: %w", err)
	}
	if pkg.Version == "" {
		return nil, errors.New("the installed package has no version")
	}
	if pkg.Engines.Node != "" && !nodeSatisfies(nodeVersion, pkg.Engines.Node) {
		return nil, fmt.Errorf("dsh %s needs Node %s, but the app runs Node %s: update the app", pkg.Version, pkg.Engines.Node, nodeVersion)
	}
	if err := checkBin(ctx, s.Node, filepath.Join(pkgDir, "lib", "bin.js"), pkg.Version); err != nil {
		return nil, err
	}

	manifest := Manifest{DSH: pkg.Version, Node: nodeVersion, Source: "npm", InstalledAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeJSON(filepath.Join(staging, manifestName), manifest); err != nil {
		return nil, err
	}

	id := "dsh-" + pkg.Version
	dest := filepath.Join(s.Dir, id)
	if _, err := os.Stat(dest); err == nil {
		if _, err := s.open(id); err != nil {
			return nil, fmt.Errorf("a broken runtime already uses %s: %w", id, err)
		}
		_ = os.RemoveAll(staging)
		installed = true
	} else if err := os.Rename(staging, dest); err != nil {
		return nil, err
	} else {
		installed = true
	}
	if activate {
		if err := s.Activate(id); err != nil {
			return nil, err
		}
	}
	rt, err := s.open(id)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

// Activate selects the installed runtime id.
func (s *Store) Activate(id string) error {
	if _, err := s.open(id); err != nil {
		return err
	}
	sel, err := s.readSelection()
	if err != nil {
		sel = selection{}
	}
	if sel.Active != id {
		sel.Previous = sel.Active
		sel.Active = id
	}
	return s.writeSelection(sel)
}

// Deactivate selects the bundled runtime.
func (s *Store) Deactivate() error {
	sel, err := s.readSelection()
	if err != nil {
		sel = selection{}
	}
	if sel.Active != "" {
		sel.Previous = sel.Active
		sel.Active = ""
	}
	return s.writeSelection(sel)
}

// Rollback selects the previous runtime again.
func (s *Store) Rollback() (*Runtime, error) {
	sel, err := s.readSelection()
	if err != nil {
		return nil, err
	}
	if sel.Previous == "" {
		if sel.Active == "" {
			return nil, errors.New("no previous runtime to roll back to")
		}
		// The previous runtime was the bundled one.
		sel.Previous, sel.Active = sel.Active, ""
		if err := s.writeSelection(sel); err != nil {
			return nil, err
		}
		return nil, nil
	}
	rt, err := s.open(sel.Previous)
	if err != nil {
		return nil, err
	}
	sel.Active, sel.Previous = sel.Previous, sel.Active
	if err := s.writeSelection(sel); err != nil {
		return nil, err
	}
	return rt, nil
}

// Remove deletes an installed runtime. The active runtime must be switched
// away first; removing the previous one clears that slot too.
func (s *Store) Remove(id string) error {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("invalid runtime id %q", id)
	}
	if _, err := s.open(id); err != nil {
		return err
	}
	sel, err := s.readSelection()
	if err != nil {
		sel = selection{}
	}
	if sel.Active == id {
		return fmt.Errorf("runtime %s is active; select another first (`dsh-gtk runtime off` for the bundled one)", id)
	}
	if sel.Previous == id {
		sel.Previous = ""
		if err := s.writeSelection(sel); err != nil {
			return err
		}
	}
	return os.RemoveAll(filepath.Join(s.Dir, id))
}

// Prune removes installed runtimes other than active, previous and the keep
// newest ones. It also removes staging directories left by an interrupted
// install.
func (s *Store) Prune(keep int) error {
	if keep < 0 {
		keep = 0
	}
	sel, err := s.readSelection()
	if err != nil {
		sel = selection{}
	}
	list, err := s.List()
	if err != nil {
		return err
	}
	sort.Slice(list, func(i, j int) bool { return newer(list[i].Dir, list[j].Dir) })
	kept := map[string]bool{sel.Active: true, sel.Previous: true}
	extra := 0
	var remove []string
	for _, rt := range list {
		if kept[rt.ID] {
			continue
		}
		if extra < keep {
			extra++
			continue
		}
		remove = append(remove, rt.Dir)
	}
	for _, dir := range remove {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".staging-") {
			continue
		}
		if st, err := e.Info(); err == nil && time.Since(st.ModTime()) > time.Hour {
			_ = os.RemoveAll(filepath.Join(s.Dir, e.Name()))
		}
	}
	return nil
}

// NodeVersion returns the version of the store's node, without the leading
// "v".
func (s *Store) NodeVersion() (string, error) {
	if s.Node == "" {
		return "", errors.New("no node executable")
	}
	return nodeVersionOf(s.Node)
}

// ---- files --------------------------------------------------------------

func (s *Store) readSelection() (selection, error) {
	var sel selection
	err := readJSON(s.State, &sel)
	if errors.Is(err, os.ErrNotExist) {
		return selection{}, nil
	}
	if err != nil {
		return selection{}, fmt.Errorf("read %s: %w", s.State, err)
	}
	return sel, nil
}

func (s *Store) writeSelection(sel selection) error {
	return writeJSON(s.State, sel)
}

func readManifest(dir string) (Manifest, error) {
	var m Manifest
	err := readJSON(filepath.Join(dir, manifestName), &m)
	if err == nil && m.DSH == "" {
		err = errors.New("manifest has no dsh version")
	}
	return m, err
}

func manifestFromPackage(dir string) (Manifest, error) {
	var pkg struct {
		Version string `json:"version"`
	}
	if err := readJSON(filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh", "package.json"), &pkg); err != nil {
		return Manifest{}, err
	}
	if pkg.Version == "" {
		return Manifest{}, errors.New("the dsh package has no version")
	}
	return Manifest{DSH: pkg.Version, Source: "bundle"}, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func newer(a, b string) bool {
	sa, ea := os.Stat(a)
	sb, eb := os.Stat(b)
	if ea != nil || eb != nil {
		return a > b
	}
	return sa.ModTime().After(sb.ModTime())
}

// ---- node ---------------------------------------------------------------

func nodeVersionOf(node string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("run %s --version: %w", node, err)
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "v"), nil
}

func checkBin(ctx context.Context, node, bin, want string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, bin, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("the installed dsh does not start: %w\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if got != want {
		return fmt.Errorf("the installed dsh reports %q, want %q", got, want)
	}
	return nil
}

// nodeSatisfies checks a version against the forms engines.node uses that
// matter here; an unknown form passes so an exotic engine does not block an
// update.
func nodeSatisfies(version, spec string) bool {
	v, ok := parseVersion(version)
	if !ok {
		return true
	}
	spec = strings.TrimSpace(spec)
	switch {
	case strings.HasPrefix(spec, ">="):
		s, ok := parseVersion(strings.TrimSpace(spec[2:]))
		return !ok || compare(v, s) >= 0
	case strings.HasPrefix(spec, "^"):
		s, ok := parseVersion(strings.TrimSpace(spec[1:]))
		return !ok || (v[0] == s[0] && compare(v, s) >= 0)
	case strings.HasPrefix(spec, "~"):
		s, ok := parseVersion(strings.TrimSpace(spec[1:]))
		return !ok || (v[0] == s[0] && v[1] == s[1] && compare(v, s) >= 0)
	default:
		s, ok := parseVersion(spec)
		return !ok || compare(v, s) >= 0
	}
}

func parseVersion(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) == 0 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		if i >= len(out) {
			break
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

func compare(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// tailWriter keeps the last lines written to it, for error messages.
type tailWriter struct {
	max   int
	lines []string
	buf   strings.Builder
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.buf.Write(p)
	for {
		s := t.buf.String()
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			break
		}
		t.lines = append(t.lines, strings.TrimRight(s[:i], "\r"))
		if len(t.lines) > t.max {
			t.lines = t.lines[len(t.lines)-t.max:]
		}
		t.buf.Reset()
		t.buf.WriteString(s[i+1:])
	}
	return len(p), nil
}
