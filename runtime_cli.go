package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/baka-world/dsh-gtk/internal/runtime"
	"github.com/egoist/mygo"
)

// runRuntimeCommand handles `dsh-gtk runtime ...`, which runs without a
// window so that the app can install and select runtimes from a terminal.
func runRuntimeCommand(args []string) int {
	mygo.App.SetName(appName)
	if len(args) == 0 {
		runtimeUsage()
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "status":
		return runtimeStatus(rest)
	case "list", "ls":
		return runtimeList(rest)
	case "check":
		return runtimeCheck(rest)
	case "install":
		return runtimeInstall(rest)
	case "use", "activate":
		return runtimeUse(rest)
	case "rollback":
		return runtimeRollback(rest)
	case "off", "bundled":
		return runtimeOff(rest)
	case "remove", "rm":
		return runtimeRemove(rest)
	case "prune":
		return runtimePrune(rest)
	case "help", "-h", "--help":
		runtimeUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "dsh-gtk runtime: unknown command %q\n\n", cmd)
		runtimeUsage()
		return 2
	}
}

func runtimeUsage() {
	fmt.Fprint(os.Stderr, `usage: dsh-gtk runtime <command>

Runtimes are dsh trees installed outside the app, updated independently of
it. The app uses the selected runtime, then the bundled one.

commands:
  status                    show the selection, the bundled runtime and node
  list                      list installed runtimes
  check [--channel X] [--cached] [--exit-code]
                            ask npm for the channel's version and say whether
                            it is newer than the runtime the app would use
                            (default: the channel that runtime came from)
  install [--no-activate] [--timeout 15m] [spec]
                            install @deepseek-ai/dsh@spec with npm and select
                            it (default: the channel's latest)
  use <id|version>          select an installed runtime
  rollback                  select the previous runtime again
  off                       use the bundled runtime
  remove <id|version>       delete an installed runtime (not the active one)
  prune [--keep N]          remove runtimes other than active, previous and
                            the N newest (default 1)

The app picks the selection up on Server > Restart.
`)
}

func runtimeStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	res, _ := mygo.App.Path(mygo.PathResources)
	if bundled, err := runtime.Bundled(res); err == nil {
		fmt.Fprintf(w, "bundled:\tdsh %s on node %s\t%s\n", bundled.DSH, orDash(bundled.NodeVersion), bundled.Dir)
	} else {
		fmt.Fprintf(w, "bundled:\tnone\t%v\n", err)
	}
	active, aerr := st.Active()
	if aerr != nil {
		fmt.Fprintf(w, "active:\tinvalid\t%v\n", aerr)
	} else if active != nil {
		fmt.Fprintf(w, "active:\tdsh %s\t%s\n", active.DSH, active.Dir)
	} else {
		fmt.Fprintf(w, "active:\tbundled\t\n")
	}
	if prev, err := st.Previous(); err == nil && prev != nil {
		fmt.Fprintf(w, "previous:\tdsh %s\t%s\n", prev.DSH, prev.Dir)
	} else {
		fmt.Fprintf(w, "previous:\tnone\t\n")
	}
	if v, err := st.NodeVersion(); err == nil {
		fmt.Fprintf(w, "node:\tv%s\t%s\n", v, st.Node)
	} else {
		fmt.Fprintf(w, "node:\tmissing\t%s\n", st.Node)
	}
	if st.NPM != "" {
		fmt.Fprintf(w, "npm:\t%s\t\n", st.NPM)
	} else {
		fmt.Fprintf(w, "npm:\tmissing\trun scripts/prepare-runtime.sh\n")
	}
	if u, err := st.CachedUpdate(); err == nil && u != nil {
		current, _ := currentRuntime(st)
		state := "up to date"
		if runtime.IsNewer(u.Version, current) {
			state = "update available: dsh-gtk runtime install " + u.Version
		}
		fmt.Fprintf(w, "update:\t%s -> %s, checked %s (%s)\t\n", u.Channel, u.Version, age(u.CheckedAt), state)
	} else {
		fmt.Fprintf(w, "update:\tunknown\trun `dsh-gtk runtime check`\n")
	}
	runtimes, lerr := st.List()
	fmt.Fprintf(w, "installed:\t%d in %s\t\n", len(runtimes), st.Dir)
	if lerr != nil {
		fmt.Fprintf(os.Stderr, "runtime: list: %v\n", lerr)
	}
	for _, rt := range runtimes {
		mark := " "
		if active != nil && rt.ID == active.ID {
			mark = "*"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\n", mark, rt.ID, rt.Dir)
	}
	return 0
}

func runtimeList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	runtimes, err := st.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	active, _ := st.Active()
	prev, _ := st.Previous()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, rt := range runtimes {
		mark := ""
		switch {
		case active != nil && rt.ID == active.ID:
			mark = "*"
		case prev != nil && rt.ID == prev.ID:
			mark = "r"
		}
		fmt.Fprintf(w, "%s\tdsh %s\t%s\n", mark, rt.DSH, rt.Dir)
	}
	w.Flush()
	if len(runtimes) == 0 {
		fmt.Fprintln(os.Stderr, "no installed runtimes; install one with `dsh-gtk runtime install latest`")
	}
	return 0
}

func runtimeCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	channel := fs.String("channel", updateChannel(), "dist-tag to check; empty follows the runtime you run")
	cached := fs.Bool("cached", false, "use the last check without asking the registry")
	exitCode := fs.Bool("exit-code", false, "exit with 10 when a newer runtime is available")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	current, label := currentRuntime(st)
	var u runtime.Update
	if *cached {
		last, err := st.CachedUpdate()
		if err != nil {
			fmt.Fprintln(os.Stderr, "runtime:", err)
			return 1
		}
		if last == nil {
			fmt.Fprintln(os.Stderr, "no cached check; run `dsh-gtk runtime check`")
			return 1
		}
		u = *last
	} else {
		var err error
		if u, err = st.CheckForUpdate(context.Background(), *channel, current); err != nil {
			fmt.Fprintln(os.Stderr, "check:", err)
			return 1
		}
	}
	newer := runtime.IsNewer(u.Version, current)
	fmt.Printf("current: dsh %s (%s)\n", orDash(current), label)
	fmt.Printf("channel: %s -> %s, checked %s\n", u.Channel, u.Version, age(u.CheckedAt))
	if newer {
		fmt.Printf("update available: dsh %s; install with `dsh-gtk runtime install %s`\n", u.Version, u.Version)
		if *exitCode {
			return 10
		}
	} else {
		fmt.Println("up to date")
	}
	return 0
}

func runtimeInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	noActivate := fs.Bool("no-activate", false, "install without selecting the runtime")
	timeout := fs.Duration("timeout", 15*time.Minute, "how long npm may take")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	spec := "latest"
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	st.Log = os.Stderr
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if fs.NArg() > 0 {
		spec = fs.Arg(0)
	} else {
		// No spec: install what the channel of the running tree names, the
		// version `runtime check` would offer. "latest" trails dsh's own
		// prereleases, so following it blindly would install an older tree
		// than the one the app ships.
		current, _ := currentRuntime(st)
		if up, err := st.CheckForUpdate(ctx, updateChannel(), current); err == nil {
			spec = up.Version
		}
	}
	fmt.Fprintf(os.Stderr, "installing @deepseek-ai/dsh@%s with %s\n", spec, st.NPM)
	rt, err := st.Install(ctx, spec, !*noActivate)
	if err != nil {
		fmt.Fprintln(os.Stderr, "install:", err)
		return 1
	}
	fmt.Printf("installed dsh %s at %s\n", rt.DSH, rt.Dir)
	if *noActivate {
		fmt.Printf("not selected; select it with `dsh-gtk runtime use %s`\n", rt.ID)
	} else {
		fmt.Println("selected; restart the app (Server > Restart) to use it")
	}
	return 0
}

func runtimeUse(args []string) int {
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: dsh-gtk runtime use <id|version>")
		return 2
	}
	id := fs.Arg(0)
	if !strings.HasPrefix(id, "dsh-") {
		id = "dsh-" + id
	}
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	if err := st.Activate(id); err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	fmt.Printf("selected %s (previous kept for rollback)\n", id)
	return 0
}

func runtimeRollback(args []string) int {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	rt, err := st.Rollback()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	if rt == nil {
		fmt.Println("selected the bundled runtime")
	} else {
		fmt.Printf("selected %s\n", rt.ID)
	}
	return 0
}

func runtimeOff(args []string) int {
	fs := flag.NewFlagSet("off", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	if err := st.Deactivate(); err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	fmt.Println("using the bundled runtime")
	return 0
}

func runtimeRemove(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: dsh-gtk runtime remove <id|version>")
		return 2
	}
	id := fs.Arg(0)
	if !strings.HasPrefix(id, "dsh-") {
		id = "dsh-" + id
	}
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	if err := st.Remove(id); err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	fmt.Printf("removed %s\n", id)
	return 0
}

func runtimePrune(args []string) int {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	keep := fs.Int("keep", 1, "how many runtimes besides active and previous to keep")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := newRuntimeStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtime:", err)
		return 1
	}
	before, _ := st.List()
	if err := st.Prune(*keep); err != nil {
		fmt.Fprintln(os.Stderr, "prune:", err)
		return 1
	}
	after, _ := st.List()
	fmt.Printf("pruned %d runtime(s); %d left\n", len(before)-len(after), len(after))
	return 0
}

// updateChannel is the dist-tag the app follows for update checks. Empty means
// "the channel of the tree we run" (see runtime.PickChannel), which is what
// dsh's own prereleases need: "latest" trails "alpha" and "next", so a fixed
// default would call an up-to-date prerelease old, or hide the next one.
// DSH_GTK_CHANNEL pins a tag instead.
func updateChannel() string {
	return strings.TrimSpace(os.Getenv("DSH_GTK_CHANNEL"))
}

// currentRuntime is the version the app would run now, and where it comes
// from.
func currentRuntime(st *runtime.Store) (version, label string) {
	if rt, err := st.Active(); err == nil && rt != nil {
		return rt.DSH, "installed " + rt.ID
	}
	resources, _ := mygo.App.Path(mygo.PathResources)
	if b, err := runtime.Bundled(resources); err == nil {
		return b.DSH, "bundled"
	}
	return "", "unknown"
}

func age(stamp string) string {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return stamp
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// newRuntimeStore builds the store of installed runtimes, and the node and
// npm it installs with: DSH_GTK_NODE / DSH_GTK_NPM, then the bundled ones,
// then node on PATH.
func newRuntimeStore() (*runtime.Store, error) {
	userData, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return nil, err
	}
	node, npm := runtimeTooling()
	return runtime.NewStore(userData, node, npm), nil
}

func runtimeTooling() (node, npm string) {
	resources, _ := mygo.App.Path(mygo.PathResources)
	node = os.Getenv("DSH_GTK_NODE")
	if node == "" {
		for _, name := range []string{"node", "node.exe"} {
			if p := filepath.Join(resources, "runtime", name); isExecutableFile(p) {
				node = p
				break
			}
		}
	}
	if node == "" {
		if p, err := exec.LookPath("node"); err == nil {
			node = p
		}
	}
	npm = os.Getenv("DSH_GTK_NPM")
	if npm == "" {
		if p := filepath.Join(resources, "runtime", "npm", "bin", "npm-cli.js"); isFile(p) {
			npm = p
		}
	}
	return node, npm
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func isExecutableFile(p string) bool {
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	return st.Mode().Perm()&0o111 != 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
