package titlebar

import (
	"strings"
	"testing"
)

func TestEnabled(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", true},
		{"1", true},
		{"on", true},
		{"0", false},
		{"off", false},
		{"false", false},
		{"NO", false},
		{" off ", false},
	} {
		t.Setenv("DSH_GO_TITLEBAR", tc.value)
		if got := Enabled(); got != tc.want {
			t.Errorf("Enabled() with DSH_GO_TITLEBAR=%q = %v; want %v", tc.value, got, tc.want)
		}
	}
}

// TestGtkCSS pins what GTK 3's theme gets wrong next to libadwaita: the
// corner radius, the states that stay square, the flat header bar and the
// 24px title buttons.
func TestGtkCSS(t *testing.T) {
	for _, want := range []string{
		"decoration {",
		"border-radius: 12px",
		".maximized decoration",
		".tiled decoration",
		".csd.popup decoration",
		"tooltip.csd decoration",
		"headerbar, .titlebar:not(headerbar)",
		"background-image: none",
		"button.titlebutton",
		"min-width: 24px",
		"border-radius: 9999px",
		"alpha(",
	} {
		if !strings.Contains(GtkCSS(false), want) {
			t.Errorf("GtkCSS(false) is missing %q", want)
		}
	}
}

// TestGtkCSSAppearance checks both appearances carry their own fills: the
// bar continues the dsh sidebar, so it must not fall back to GTK's colours.
func TestGtkCSSAppearance(t *testing.T) {
	light, dark := GtkCSS(false), GtkCSS(true)
	for _, want := range []string{
		"background-color: " + lightFill + ";",
		"color: " + lightText + ";",
	} {
		if !strings.Contains(light, want) {
			t.Errorf("GtkCSS(false) is missing %q", want)
		}
	}
	for _, want := range []string{
		"background-color: " + darkFill + ";",
		"color: " + darkText + ";",
	} {
		if !strings.Contains(dark, want) {
			t.Errorf("GtkCSS(true) is missing %q", want)
		}
	}
	if strings.Contains(light, "background-color: "+darkFill+";") {
		t.Errorf("GtkCSS(false) uses the dark fill %s", darkFill)
	}
	if strings.Contains(dark, "background-color: "+lightFill+";") {
		t.Errorf("GtkCSS(true) uses the light fill %s", lightFill)
	}
}
