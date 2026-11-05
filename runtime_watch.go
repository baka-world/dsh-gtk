package main

import (
	"fmt"
	"log"
	"time"

	"github.com/baka-world/dsh-gtk/internal/dsh"
	dshruntime "github.com/baka-world/dsh-gtk/internal/runtime"
	"github.com/egoist/mygo"
)

// runtimeWatchPeriod is how often the selection file is compared with the
// runtime the server runs. The file is a few bytes and the check is a stat
// plus a read, so a short period is cheaper than watching it.
const runtimeWatchPeriod = 2 * time.Second

// watchRuntimeSelection offers a restart when the selection changes under a
// running server.
//
// dsh runs as a child of the app, so `dsh-gtk runtime install` — or a
// runtime installed and left for "Later" in the dialog — writes the selection
// and nothing else: the window keeps serving the old tree until the next
// launch. Without this the update looks applied while `dsh --version` in the
// conversation still answers with the old one.
func (l *Launcher) watchRuntimeSelection() {
	t := time.NewTicker(runtimeWatchPeriod)
	defer t.Stop()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-t.C:
			if p, ok := l.pendingRuntimeChange(); ok {
				l.offerRuntimeRestart(p)
			}
		}
	}
}

// restartPrompt is a selection that differs from the running runtime.
type restartPrompt struct {
	// selection is the runtime id, empty when the bundle is selected.
	selection string
	// version is the dsh version of that selection.
	version string
	// running is the dsh version the server runs.
	running string
}

// pendingRuntimeChange reports whether the selection names a runtime other
// than the running one, and remembers the selection so one change prompts
// once: "Later" stays quiet until something else is selected.
func (l *Launcher) pendingRuntimeChange() (restartPrompt, bool) {
	l.mu.Lock()
	managed, runningID, runningDSH := l.managed, l.runningID, l.runningDSH
	busy := l.starting || l.installing || l.quitting
	asked := l.promptedSelection
	l.mu.Unlock()
	if !managed || busy {
		return restartPrompt{}, false
	}

	rt, err := activeSelection()
	if err != nil {
		// A selection that no longer opens is not a restart anyone wants;
		// boot falls back to the bundle and logs it.
		return restartPrompt{}, false
	}
	id, version := "", ""
	if rt != nil {
		id, version = rt.ID, rt.DSH
	}
	if id == runningID {
		// Back to what runs: a later change prompts again.
		l.mu.Lock()
		l.promptedSelection = ""
		l.mu.Unlock()
		return restartPrompt{}, false
	}
	if version == "" {
		version = id
	}
	l.mu.Lock()
	already := asked == id
	l.promptedSelection = id
	l.mu.Unlock()
	if already {
		return restartPrompt{}, false
	}
	return restartPrompt{selection: id, version: version, running: runningDSH}, true
}

// offerRuntimeRestart asks whether to restart the server on the new runtime.
func (l *Launcher) offerRuntimeRestart(p restartPrompt) {
	detail := "The server still runs " + orDash(p.running) + ".\n\n" +
		"Restart it to run the selected runtime. The conversation keeps its history; " +
		"a turn that is running now is interrupted."
	if p.selection != "" {
		detail += "\n\nSelected: " + p.selection
	}
	message := "A different dsh runtime is selected."
	if p.version != "" {
		message = fmt.Sprintf("dsh %s is selected.", p.version)
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Type:    mygo.MessageQuestion,
		Title:   "Runtime Changed",
		Message: message,
		Detail:  detail,
		Buttons: []string{"Restart", "Later"},
	})
	if err != nil {
		log.Printf("dsh-gtk: runtime change prompt: %v", err)
		return
	}
	if res.Button == 0 {
		log.Printf("dsh-gtk: restarting for the selected runtime %s", p.version)
		go l.start()
	}
}

// noteRunning records which dsh tree the server starts from, so the watcher
// can tell when the selection moves away from it. A dsh from DSH_GTK_DSH, or
// from PATH, is not the store's to manage: no prompt then.
func (l *Launcher) noteRunning(resources string, launch dsh.Launch, installed *dshruntime.Runtime) {
	managed := launch.Source == "bundle" || launch.Source == "installed"
	id, version := "", ""
	if managed {
		if b, err := dshruntime.Bundled(resources); err == nil {
			version = b.DSH
		}
		if installed != nil {
			id, version = installed.ID, installed.DSH
		}
	}
	l.mu.Lock()
	l.managed, l.runningID, l.runningDSH = managed, id, version
	l.mu.Unlock()
	if managed {
		log.Printf("dsh-gtk: runtime watched: %s %s", orDash(id), orDash(version))
	}
}
