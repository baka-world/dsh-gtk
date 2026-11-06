package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUserCSS pins the experiment hook: a stylesheet named by DSH_GTK_CSS is
// injected into every page, so trying a layout change costs a restart instead
// of a rebuild.
func TestUserCSS(t *testing.T) {
	if got := userCSS(); got != "" {
		t.Fatalf("userCSS() without the variable = %q; want empty", got)
	}

	path := filepath.Join(t.TempDir(), "page.css")
	css := "/* a rule with a \"quote\" */\n.wSkVaW_body { --x: 1px; }"
	if err := os.WriteFile(path, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSH_GTK_CSS", path)

	got := userCSS()
	if !strings.Contains(got, "data-dsh-user-css") || !strings.Contains(got, "style.textContent") {
		t.Fatalf("userCSS() = %q; want a style element", got)
	}
	if !strings.Contains(got, `\"quote\"`) {
		t.Errorf("userCSS() did not escape the CSS for a JS string: %q", got)
	}
}

func TestUserCSSMissingFile(t *testing.T) {
	t.Setenv("DSH_GTK_CSS", filepath.Join(t.TempDir(), "nope.css"))
	if got := userCSS(); got != "" {
		t.Errorf("userCSS() with a missing file = %q; want empty", got)
	}
}
