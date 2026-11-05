package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/baka-world/dsh-gtk/internal/dsh"
	dshruntime "github.com/baka-world/dsh-gtk/internal/runtime"
)

// bundledResources writes the smallest resources/runtime that runtime.Bundled
// accepts, so noteRunning can read a version.
func bundledResources(t *testing.T, version string) string {
	t.Helper()
	res := t.TempDir()
	dir := filepath.Join(res, "runtime")
	bin := filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh", "lib")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "bin.js"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"dsh": "` + version + `", "source": "bundle"}`
	if err := os.WriteFile(filepath.Join(dir, "runtime.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return res
}

// TestNoteRunning pins what the watcher compares with the selection: the
// bundled tree is id "", an installed one is its directory name, and a dsh
// that came from the environment is not the store's to manage.
func TestNoteRunning(t *testing.T) {
	res := bundledResources(t, "1.2.3")

	tests := []struct {
		name      string
		launch    dsh.Launch
		installed *dshruntime.Runtime
		managed   bool
		id, dsh   string
	}{
		{"bundle", dsh.Launch{Source: "bundle"}, nil, true, "", "1.2.3"},
		{"installed", dsh.Launch{Source: "installed"},
			&dshruntime.Runtime{ID: "dsh-2.0.0", DSH: "2.0.0"}, true, "dsh-2.0.0", "2.0.0"},
		{"env", dsh.Launch{Source: "env"}, nil, false, "", ""},
		{"path", dsh.Launch{Source: "PATH"}, nil, false, "", ""},
	}
	for _, tt := range tests {
		l := &Launcher{}
		l.noteRunning(res, tt.launch, tt.installed)
		if l.managed != tt.managed || l.runningID != tt.id || l.runningDSH != tt.dsh {
			t.Errorf("%s: managed=%v id=%q dsh=%q; want %v %q %q",
				tt.name, l.managed, l.runningID, l.runningDSH, tt.managed, tt.id, tt.dsh)
		}
	}
}

// TestNoteRunningUnknownBundle keeps the watcher quiet when the bundled
// runtime carries no manifest: no version to show, and nothing to compare.
func TestNoteRunningUnknownBundle(t *testing.T) {
	l := &Launcher{}
	l.noteRunning(t.TempDir(), dsh.Launch{Source: "bundle"}, nil)
	if !l.managed || l.runningDSH != "" {
		t.Errorf("managed=%v dsh=%q", l.managed, l.runningDSH)
	}
}
