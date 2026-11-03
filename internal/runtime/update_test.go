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
	u, err := s.CheckForUpdate(context.Background(), "alpha")
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

func TestCheckForUpdateDefaultChannel(t *testing.T) {
	s := newFakeStore(t)
	u, err := s.CheckForUpdate(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if u.Channel != "latest" || u.Version != "1.2.3" {
		t.Fatalf("Update = %+v", u)
	}
}

func TestCheckForUpdateMissingChannel(t *testing.T) {
	s := newFakeStore(t)
	_, err := s.CheckForUpdate(context.Background(), "beta")
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
	_, err := s.CheckForUpdate(context.Background(), "latest")
	if err == nil || !strings.Contains(err.Error(), "registry offline") {
		t.Fatalf("err = %v; want npm's message", err)
	}
}

func TestCheckForUpdateNeedsTooling(t *testing.T) {
	s := &Store{}
	if _, err := s.CheckForUpdate(context.Background(), "latest"); err == nil {
		t.Fatal("a store without node or npm checked for updates")
	}
}
