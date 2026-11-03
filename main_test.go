package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

func TestRendererFromEnv(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  mygo.DMABUFRenderer
	}{
		{"", mygo.DMABUFRendererAuto},
		{"1", mygo.DMABUFRendererOn},
		{" ON ", mygo.DMABUFRendererOn},
		{"dmabuf", mygo.DMABUFRendererOn},
		{"0", mygo.DMABUFRendererOff},
		{"software", mygo.DMABUFRendererOff},
		{"maybe", mygo.DMABUFRendererAuto},
	} {
		if got := rendererFromEnv(tc.value); got != tc.want {
			t.Errorf("rendererFromEnv(%q) = %q; want %q", tc.value, got, tc.want)
		}
	}
}

func TestPreloadScript(t *testing.T) {
	script := preloadScript()
	for _, want := range []string{"__dgoCorners", "__dshGoWarmup"} {
		if !strings.Contains(script, want) {
			t.Errorf("preloadScript misses %q", want)
		}
	}
	t.Setenv("DSH_GTK_WARMUP", "0")
	if script := preloadScript(); strings.Contains(script, "__dshGoWarmup") {
		t.Error("preloadScript keeps the warm-up with DSH_GTK_WARMUP=0")
	}
	t.Setenv("DSH_GTK_TITLEBAR", "0")
	if got := preloadScript(); got != "" {
		t.Errorf("preloadScript() with both off = %q", got)
	}
}
