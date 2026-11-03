// Package dsh locates the DeepSeek Harness CLI and supervises the web
// server a MyGo window shows.
package dsh

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Launch is how to start the dsh CLI: an executable, and the arguments that
// come before the dsh command line.
type Launch struct {
	// Executable is the program to run: a node binary, or the dsh command
	// itself when it is an executable.
	Executable string
	// Args precede the dsh arguments, e.g. the path of lib/bin.js.
	Args []string
	// Source says where the dsh tree was found, for diagnostics: "env",
	// "installed", "bundle" or "PATH".
	Source string
	// NodeDir is the directory of the interpreter, which the server puts
	// first on PATH so that the helpers dsh starts use it too.
	NodeDir string
}

// Command returns the program and arguments of a dsh command line.
func (l Launch) Command(args ...string) (string, []string) {
	argv := append(append([]string(nil), l.Args...), args...)
	if runtime.GOOS == "windows" {
		switch strings.ToLower(filepath.Ext(l.Executable)) {
		case ".cmd", ".bat":
			return "cmd.exe", append([]string{"/c", l.Executable}, argv...)
		}
	}
	return l.Executable, argv
}

// DiscoverOptions overrides what Discover reads, for tests and for callers
// that resolve paths differently.
type DiscoverOptions struct {
	// Resources is the app's resource directory (mygo.PathResources).
	Resources string
	// InstalledDir is the selected runtime installed outside the app; its
	// node_modules/@deepseek-ai/dsh wins over the bundled tree.
	InstalledDir string
	// InstalledNode is a node executable inside InstalledDir, empty when
	// the runtime uses the app's node.
	InstalledNode string
	// DSH overrides DSH_GO_DSH, a path to lib/bin.js or to an executable
	// dsh.
	DSH string
	// Node overrides DSH_GO_NODE, the interpreter of a JavaScript entry
	// point.
	Node string
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
	// LookPath defaults to exec.LookPath.
	LookPath func(string) (string, error)
}

// Discover finds the dsh runtime, in this order:
//
//  1. DSH_GO_DSH, a path to lib/bin.js or to an executable dsh;
//  2. InstalledDir, a runtime installed by `dsh-go runtime install`;
//  3. the bundle under resources/runtime, written by scripts/prepare-runtime.sh;
//  4. a dsh command on PATH.
//
// A JavaScript entry point runs on DSH_GO_NODE, a node inside the installed
// runtime, the bundled node, or the node on PATH, in that order.
func Discover(o DiscoverOptions) (Launch, error) {
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	if o.Resources == "" {
		return Launch{}, errors.New("dsh: no resource directory")
	}

	bundle := filepath.Join(o.Resources, "runtime")
	nodeName := "node"
	if runtime.GOOS == "windows" {
		nodeName = "node.exe"
	}
	bundledNode := filepath.Join(bundle, nodeName)
	bundledBin := filepath.Join(bundle, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")

	nodeOverride := o.Node
	if nodeOverride == "" {
		nodeOverride = o.Getenv("DSH_GO_NODE")
	}
	dshOverride := o.DSH
	if dshOverride == "" {
		dshOverride = o.Getenv("DSH_GO_DSH")
	}

	// node returns the interpreter for a JavaScript entry point: the one
	// the environment names, the runtime's own, the bundled one, or the one
	// on PATH.
	node := func(entry, source string) (Launch, error) {
		if nodeOverride != "" {
			if !isFile(nodeOverride) {
				return Launch{}, fmt.Errorf("node %q does not exist", nodeOverride)
			}
			return Launch{Executable: nodeOverride, Args: []string{entry}, Source: source, NodeDir: filepath.Dir(nodeOverride)}, nil
		}
		if isExecutable(o.InstalledNode) {
			return Launch{Executable: o.InstalledNode, Args: []string{entry}, Source: source, NodeDir: filepath.Dir(o.InstalledNode)}, nil
		}
		if isExecutable(bundledNode) {
			return Launch{Executable: bundledNode, Args: []string{entry}, Source: source, NodeDir: bundle}, nil
		}
		p, err := o.LookPath(nodeName)
		if err != nil {
			return Launch{}, fmt.Errorf("run %s: no bundled node at %s and no %s on PATH: %w", entry, bundledNode, nodeName, err)
		}
		return Launch{Executable: p, Args: []string{entry}, Source: source}, nil
	}

	if dshOverride != "" {
		if !isFile(dshOverride) {
			return Launch{}, fmt.Errorf("dsh %q does not exist", dshOverride)
		}
		if isScript(dshOverride) {
			return node(dshOverride, "env")
		}
		if runtime.GOOS != "windows" && !isExecutable(dshOverride) {
			return Launch{}, fmt.Errorf("dsh %q is not executable", dshOverride)
		}
		return Launch{Executable: dshOverride, Source: "env"}, nil
	}

	if o.InstalledDir != "" {
		bin := filepath.Join(o.InstalledDir, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
		if isFile(bin) {
			return node(bin, "installed")
		}
	}

	if isFile(bundledBin) {
		return node(bundledBin, "bundle")
	}

	if p, err := o.LookPath("dsh"); err == nil {
		return Launch{Executable: p, Source: "PATH"}, nil
	}

	return Launch{}, fmt.Errorf(
		"no dsh runtime found: run scripts/prepare-runtime.sh to bundle one, install it with `dsh-go runtime install latest`, or set DSH_GO_DSH to a dsh executable or lib/bin.js",
	)
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func isExecutable(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || st.Mode().Perm()&0o111 != 0
}

func isScript(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".js", ".mjs", ".cjs":
		return true
	}
	return false
}
