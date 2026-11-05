package fps

import (
	"strings"
	"testing"
)

func TestEnabled(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"off", false},
		{"false", false},
		{"no", false},
		{"1", true},
		{"on", true},
		{"true", true},
		{" ON ", true},
	}
	for _, tt := range tests {
		t.Setenv("DSH_GTK_FPS", tt.env)
		if got := Enabled(); got != tt.want {
			t.Errorf("Enabled() with DSH_GTK_FPS=%q = %v; want %v", tt.env, got, tt.want)
		}
	}
}

// TestScriptGuards pins the properties the preload relies on: it must be
// idempotent (a reload and the page's own navigation run it again), it must
// stay out of subframes and of the mygo:// pages, and it must measure with
// requestAnimationFrame.
func TestScriptGuards(t *testing.T) {
	for _, want := range []string{
		"if (window.__dshFPS) return;",
		"window.top !== window",
		"location.protocol !== 'http:'",
		"requestAnimationFrame",
		"MutationObserver",
		"window.__dshFPS = {",
		"slow",
	} {
		if !strings.Contains(Script, want) {
			t.Errorf("Script lacks %q", want)
		}
	}
	if strings.Count(Script, "window.setInterval(sample, 250)") != 1 {
		t.Error("Script samples more than once per interval")
	}
}
