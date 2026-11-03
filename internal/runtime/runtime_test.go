package runtime

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fakeNPM = `const fs=require('fs'), path=require('path');
const args=process.argv.slice(2);
if (args[0]==='view') {
  if (process.env.FAKE_FAIL) { console.error('fake npm failed: ' + process.env.FAKE_FAIL); process.exit(1); }
  console.log(process.env.FAKE_TAGS || JSON.stringify({latest:'1.2.3',alpha:'2.0.0-alpha.1'}));
  process.exit(0);
}
if (process.env.FAKE_FAIL) { console.error('fake npm failed: ' + process.env.FAKE_FAIL); process.exit(1); }
const prefix=args[args.indexOf('--prefix')+1];
const spec=args[args.length-1];
let version=process.env.FAKE_VERSION;
if (!version) { version=spec.slice(spec.lastIndexOf('@')+1); }
const pkg=path.join(prefix,'node_modules','@deepseek-ai','dsh');
fs.mkdirSync(path.join(pkg,'lib'),{recursive:true});
fs.writeFileSync(path.join(prefix,'.npm-cache-seen'), process.env.npm_config_cache||'');
fs.writeFileSync(path.join(pkg,'package.json'), JSON.stringify({name:'@deepseek-ai/dsh',version:version,engines:{node:process.env.FAKE_ENGINE||'>=1.0.0'}}));
fs.writeFileSync(path.join(pkg,'lib','bin.js'), 'console.log(require("../package.json").version)\n');
console.log('fake npm: installed '+version);
`

func newFakeStore(t *testing.T) *Store {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	npm := filepath.Join(dir, "npm-cli.js")
	if err := os.WriteFile(npm, []byte(fakeNPM), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(filepath.Join(dir, "data"), node, npm)
	s.Log = io.Discard
	return s
}

func TestInstallActivates(t *testing.T) {
	s := newFakeStore(t)
	rt, err := s.Install(context.Background(), "0.2.2-alpha.3", true)
	if err != nil {
		t.Fatal(err)
	}
	if rt.ID != "dsh-0.2.2-alpha.3" || rt.DSH != "0.2.2-alpha.3" {
		t.Fatalf("runtime = %+v", rt)
	}
	if _, err := os.Stat(filepath.Join(rt.Dir, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")); err != nil {
		t.Fatalf("installed tree: %v", err)
	}
	active, err := s.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active == nil || active.ID != rt.ID {
		t.Fatalf("Active = %+v; want %s", active, rt.ID)
	}
	seen, err := os.ReadFile(filepath.Join(rt.Dir, ".npm-cache-seen"))
	if err != nil {
		t.Fatalf("the npm cache variable did not reach npm: %v", err)
	}
	if !strings.HasSuffix(string(seen), "npm-cache") {
		t.Errorf("npm_config_cache = %q", seen)
	}
}

func TestInstallRejectsEngineMismatch(t *testing.T) {
	s := newFakeStore(t)
	t.Setenv("FAKE_ENGINE", ">=99.0.0")
	_, err := s.Install(context.Background(), "9.0.0", true)
	if err == nil || !strings.Contains(err.Error(), "needs Node") {
		t.Fatalf("err = %v; want an engine mismatch", err)
	}
	active, _ := s.Active()
	if active != nil {
		t.Errorf("Active = %+v after a failed install", active)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("installed runtimes = %v after a failed install", list)
	}
}

func TestInstallReportsNPMFailure(t *testing.T) {
	s := newFakeStore(t)
	t.Setenv("FAKE_FAIL", "registry down")
	_, err := s.Install(context.Background(), "0.2.3", false)
	if err == nil || !strings.Contains(err.Error(), "registry down") {
		t.Fatalf("err = %v; want npm's output", err)
	}
	entries, _ := os.ReadDir(s.Dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("staging %s survived a failed install", e.Name())
		}
	}
}

func TestActivateRollbackDeactivate(t *testing.T) {
	s := newFakeStore(t)
	for _, v := range []string{"1.0.0", "2.0.0"} {
		if _, err := s.Install(context.Background(), v, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Activate("dsh-1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("dsh-2.0.0"); err != nil {
		t.Fatal(err)
	}
	prev, err := s.Previous()
	if err != nil || prev == nil || prev.ID != "dsh-1.0.0" {
		t.Fatalf("Previous = %+v, %v", prev, err)
	}
	rt, err := s.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if rt.ID != "dsh-1.0.0" {
		t.Fatalf("Rollback = %+v", rt)
	}
	active, _ := s.Active()
	prev, _ = s.Previous()
	if active == nil || active.ID != "dsh-1.0.0" || prev == nil || prev.ID != "dsh-2.0.0" {
		t.Fatalf("after rollback active=%+v previous=%+v", active, prev)
	}
	if err := s.Deactivate(); err != nil {
		t.Fatal(err)
	}
	if active, _ := s.Active(); active != nil {
		t.Fatalf("Active = %+v after Deactivate", active)
	}
	if prev, _ := s.Previous(); prev == nil || prev.ID != "dsh-1.0.0" {
		t.Fatalf("Previous = %+v after Deactivate", prev)
	}
}

func TestRollbackToBundled(t *testing.T) {
	s := newFakeStore(t)
	if _, err := s.Install(context.Background(), "1.0.0", true); err != nil {
		t.Fatal(err)
	}
	rt, err := s.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if rt != nil {
		t.Fatalf("Rollback = %+v; want the bundled runtime", rt)
	}
	if active, _ := s.Active(); active != nil {
		t.Fatalf("Active = %+v after rolling back to bundled", active)
	}
	if prev, _ := s.Previous(); prev == nil || prev.ID != "dsh-1.0.0" {
		t.Fatalf("Previous = %+v", prev)
	}
	rt, err = s.Rollback()
	if err != nil || rt == nil || rt.ID != "dsh-1.0.0" {
		t.Fatalf("second Rollback = %+v, %v", rt, err)
	}
}

func TestRemove(t *testing.T) {
	s := newFakeStore(t)
	writeRuntime(t, s, "1.0.0")
	writeRuntime(t, s, "2.0.0")
	if err := s.Activate("dsh-2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("dsh-1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("dsh-1.0.0"); err == nil {
		t.Fatal("Remove deleted the active runtime")
	}
	if err := s.Remove("dsh-2.0.0"); err != nil {
		t.Fatal(err)
	}
	if prev, err := s.Previous(); err != nil || prev != nil {
		t.Fatalf("Previous = %+v, %v after removing it", prev, err)
	}
	if list, _ := s.List(); len(list) != 1 || list[0].ID != "dsh-1.0.0" {
		t.Fatalf("List = %+v", list)
	}
}

func TestPruneKeepsSelection(t *testing.T) {
	s := newFakeStore(t)
	for _, v := range []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0"} {
		writeRuntime(t, s, v)
	}
	if err := s.Activate("dsh-2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("dsh-3.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune(0); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, rt := range list {
		ids[rt.ID] = true
	}
	if !ids["dsh-2.0.0"] || !ids["dsh-3.0.0"] || len(ids) != 2 {
		t.Fatalf("kept %v; want active and previous only", ids)
	}
}

func TestCorruptSelectionRecovers(t *testing.T) {
	s := newFakeStore(t)
	if err := os.MkdirAll(filepath.Dir(s.State), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.State, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Active(); err == nil {
		t.Fatal("Active accepted a corrupt selection")
	}
	if _, err := s.Install(context.Background(), "1.0.0", true); err != nil {
		t.Fatal(err)
	}
	active, err := s.Active()
	if err != nil || active == nil {
		t.Fatalf("Active after install = %+v, %v", active, err)
	}
}

func TestBundledReadsManifest(t *testing.T) {
	res := t.TempDir()
	dir := filepath.Join(res, "runtime")
	writeRuntimeAt(t, dir, "0.2.2-alpha.3")
	if err := os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rt, err := Bundled(res)
	if err != nil {
		t.Fatal(err)
	}
	if rt.DSH != "0.2.2-alpha.3" || !rt.Bundled || rt.NodePath != filepath.Join(dir, "node") {
		t.Fatalf("Bundled = %+v", rt)
	}
}

func TestNodeSatisfies(t *testing.T) {
	tests := []struct {
		version, spec string
		want          bool
	}{
		{"24.11.0", ">=24.9.0", true},
		{"24.8.9", ">=24.9.0", false},
		{"24.11.0", ">= 24.9.0", true},
		{"24.11.0", "^24.9.0", true},
		{"25.0.0", "^24.9.0", false},
		{"24.11.0", "~24.11.0", true},
		{"24.10.0", "~24.11.0", false},
		{"24.11.0", "24.11.0", true},
		{"24.11.0", "not a version", true},
	}
	for _, tt := range tests {
		if got := nodeSatisfies(tt.version, tt.spec); got != tt.want {
			t.Errorf("nodeSatisfies(%q, %q) = %v; want %v", tt.version, tt.spec, got, tt.want)
		}
	}
}

func TestListSkipsBrokenRuntimes(t *testing.T) {
	s := newFakeStore(t)
	writeRuntime(t, s, "1.0.0")
	if err := os.MkdirAll(filepath.Join(s.Dir, "dsh-broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "dsh-1.0.0" {
		t.Fatalf("List = %+v", list)
	}
}

func writeRuntime(t *testing.T, s *Store, version string) {
	t.Helper()
	writeRuntimeAt(t, filepath.Join(s.Dir, "dsh-"+version), version)
}

func writeRuntimeAt(t *testing.T, dir, version string) {
	t.Helper()
	pkg := filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh")
	if err := os.MkdirAll(filepath.Join(pkg, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@deepseek-ai/dsh","version":"`+version+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "lib", "bin.js"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(`{"dsh":"`+version+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}
