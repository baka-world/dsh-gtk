package warmup

import (
	"strings"
	"testing"
)

func TestEnabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", true},
		{"1", true},
		{"on", true},
		{"true", true},
		{"maybe", true},
		{"0", false},
		{"off", false},
		{"false", false},
		{"no", false},
		{" OFF ", false},
	}
	for _, tt := range tests {
		t.Setenv("DSH_GO_WARMUP", tt.value)
		if got := Enabled(); got != tt.want {
			t.Errorf("Enabled() with DSH_GO_WARMUP=%q = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestScript(t *testing.T) {
	// The script must only act on the dsh workspace, must target the
	// scrollport dsh marks for its own scroll code, and must paint the end
	// rather than hide it: a hidden jump warms nothing.
	for _, want := range []string{
		"location.protocol !== 'http:'",
		"[data-conversation-scroll]",
		"requestAnimationFrame",
		"__dshGoWarmup",
	} {
		if !strings.Contains(Script, want) {
			t.Errorf("Script does not contain %q", want)
		}
	}
}
