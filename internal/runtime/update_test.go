package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"0.2.2-alpha.3", "0.2.1-rc.5", 1, true},
		{"0.2.1-rc.5", "0.2.2-alpha.3", -1, true},
		{"0.2.1-rc.5", "0.2.1-rc.5", 0, true},
		{"1.0.0", "1.0.0-rc.1", 1, true},
		{"1.0.0-alpha.2", "1.0.0-alpha.10", -1, true},
		{"1.0.0-beta", "1.0.0-alpha", 1, true},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1, true},
		{"1.2", "1.2.0", 0, true},
		{"v1.2.3", "1.2.3", 0, true},
		{"1.2.3+build.5", "1.2.3", 0, true},
		{"not a version", "1.0.0", 0, false},
		{"1.0.0", "", 0, false},
		{"1", "0.9.9", 1, true},
	}
	for _, tt := range tests {
		got, ok := CompareVersions(tt.a, tt.b)
		if got != tt.want || ok != tt.ok {
			t.Errorf("CompareVersions(%q, %q) = %d, %v; want %d, %v", tt.a, tt.b, got, ok, tt.want, tt.ok)
		}
	}
	if !IsNewer("0.2.2-alpha.4", "0.2.2-alpha.3") {
		t.Error("IsNewer missed a later prerelease")
	}
	if IsNewer("0.2.1-rc.5", "0.2.2-alpha.3") {
		t.Error("IsNewer took an older version for a newer one")
	}
	if IsNewer("garbage", "0.2.2-alpha.3") {
		t.Error("IsNewer accepted garbage")
	}
}

func TestCheckForUpdateCaches(t *testing.T) {
	s := newFakeStore(t)
	u, err := s.CheckForUpdate(context.Background(), "alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if u.Channel != "alpha" || u.Version != "2.0.0-alpha.1" || u.CheckedAt == "" {
		t.Fatalf("Update = %+v", u)
	}
	cached, err := s.CachedUpdate()
	if err != nil || cached == nil || cached.Version != u.Version || cached.Channel != "alpha" {
		t.Fatalf("CachedUpdate = %+v, %v", cached, err)
	}
}

func TestCheckForUpdateFollowsRuntime(t *testing.T) {
	tests := []struct {
		current, channel, version string
	}{
		{"1.2.3", "latest", "1.2.3"},
		{"2.0.0-alpha.1", "alpha", "2.0.0-alpha.1"},
		{"9.9.9", "alpha", "2.0.0-alpha.1"}, // unknown tree: follow the newest tag
		{"", "alpha", "2.0.0-alpha.1"},
	}
	for _, tt := range tests {
		s := newFakeStore(t)
		u, err := s.CheckForUpdate(context.Background(), "", tt.current)
		if err != nil {
			t.Fatalf("current %q: %v", tt.current, err)
		}
		if u.Channel != tt.channel || u.Version != tt.version {
			t.Errorf("current %q: Update = %+v; want %s %s", tt.current, u, tt.channel, tt.version)
		}
	}
}

func TestPickChannel(t *testing.T) {
	dsh := map[string]string{"latest": "0.2.1-rc.5", "alpha": "0.2.2-alpha.3", "next": "0.2.2-rc.1"}
	tests := []struct {
		current, want string
	}{
		{"0.2.2-rc.1", "next"},
		{"0.2.2-alpha.3", "alpha"},
		{"0.2.1-rc.5", "latest"},
		{"0.2.3", "next"}, // nothing names it: follow the newest
		{"", "next"},
	}
	for _, tt := range tests {
		got, err := PickChannel(dsh, tt.current)
		if err != nil {
			t.Fatalf("current %q: %v", tt.current, err)
		}
		if got != tt.want {
			t.Errorf("PickChannel(%q) = %q; want %q", tt.current, got, tt.want)
		}
	}
	if got, err := PickChannel(dsh, "1.0.0"); err != nil || got != "next" {
		t.Errorf("a release older than every tag: %q, %v", got, err)
	}
	if _, err := PickChannel(nil, "1.0.0"); err == nil {
		t.Error("PickChannel accepted an empty tag set")
	}
}

func TestCheckForUpdateMissingChannel(t *testing.T) {
	s := newFakeStore(t)
	_, err := s.CheckForUpdate(context.Background(), "beta", "")
	if err == nil || !strings.Contains(err.Error(), "alpha, latest") {
		t.Fatalf("err = %v; want the available tags", err)
	}
	if cached, _ := s.CachedUpdate(); cached != nil {
		t.Errorf("a failed check was cached: %+v", cached)
	}
}

func TestCheckForUpdateFailure(t *testing.T) {
	s := newFakeStore(t)
	t.Setenv("FAKE_FAIL", "registry offline")
	_, err := s.CheckForUpdate(context.Background(), "latest", "")
	if err == nil || !strings.Contains(err.Error(), "registry offline") {
		t.Fatalf("err = %v; want npm's message", err)
	}
}

func TestCheckForUpdateNeedsTooling(t *testing.T) {
	s := &Store{}
	if _, err := s.CheckForUpdate(context.Background(), "latest", ""); err == nil {
		t.Fatal("a store without node or npm checked for updates")
	}
}
