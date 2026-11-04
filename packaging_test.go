package main

import (
	"encoding/json"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

// TestRPMLinkResolves guards the one thing a user sees first: the command the
// desktop entry runs. The spec makes /usr/bin/<slug> a relative symlink into
// /opt, and a wrong number of ".." leaves a dangling link — which makes GNOME
// drop the entry from the app grid altogether, so nothing looks installed.
func TestRPMLinkResolves(t *testing.T) {
	spec, err := os.ReadFile("packaging/rpm/dsh-gtk.spec.in")
	if err != nil {
		t.Fatal(err)
	}
	link := regexp.MustCompile(`(?m)^ln -s (\S+) %\{buildroot\}%\{_bindir\}/%\{name\}$`).FindSubmatch(spec)
	if link == nil {
		t.Fatal("no `ln -s ... %{buildroot}%{_bindir}/%{name}` line in the spec")
	}
	// make-rpm.sh fills in %{name} (the slug) and @BINARY@ (the binary mygo
	// build writes, also the slug), and the link lives in /usr/bin.
	var manifest struct {
		Slug string `json:"slug"`
	}
	raw, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	slug := manifest.Slug
	resolved := strings.NewReplacer("%{name}", slug, "@BINARY@", slug).Replace(string(link[1]))
	target := path.Join("/usr/bin", resolved)
	if want := path.Join("/opt", slug, slug); target != want {
		t.Errorf("/usr/bin/%s -> %s resolves to %s, want %s", slug, resolved, target, want)
	}
}
