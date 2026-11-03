package main

import (
	"strings"
	"testing"
)

func TestRendererChoice(t *testing.T) {
	tests := []struct {
		name     string
		override string
		wayland  bool
		gdkX11   bool
		blank    bool
		disable  bool
		reason   string
	}{
		{"wayland intel", "", true, false, false, false, "dma-buf (Wayland)"},
		{"not wayland", "", false, false, false, true, "software (not Wayland)"},
		{"x11 forced", "", true, true, false, true, "software (not Wayland)"},
		{"nvidia", "", true, false, true, true, "software (NVIDIA, VM or container)"},
		{"force on over x11", "1", false, true, true, false, "dma-buf (DSH_GTK_DMABUF)"},
		{"force on spelled", " ON ", false, false, true, false, "dma-buf (DSH_GTK_DMABUF)"},
		{"force off", "0", true, false, false, true, "software (DSH_GTK_DMABUF)"},
		{"force off spelled", "software", true, false, false, true, "software (DSH_GTK_DMABUF)"},
		{"unknown override is auto", "maybe", true, false, false, false, "dma-buf (Wayland)"},
	}
	for _, tt := range tests {
		disable, reason := rendererChoice(tt.override, tt.wayland, tt.gdkX11, tt.blank)
		if disable != tt.disable || reason != tt.reason {
			t.Errorf("%s: rendererChoice(%q, %v, %v, %v) = %v, %q; want %v, %q",
				tt.name, tt.override, tt.wayland, tt.gdkX11, tt.blank, disable, reason, tt.disable, tt.reason)
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
