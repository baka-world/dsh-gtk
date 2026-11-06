package drawer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstall pins what makes the plugin load: the files under the package
// name, the dependency that resolves it, and the patch row that inserts it.
func TestInstall(t *testing.T) {
	home := t.TempDir()
	if err := Install(home); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(home, "profiles", "web")

	client, err := os.ReadFile(filepath.Join(profile, "node_modules", Name, "lib", "client.js"))
	if err != nil {
		t.Fatalf("client.js: %v", err)
	}
	if !strings.Contains(string(client), "__ModuleLoader__.load") {
		t.Error("client.js is not a client plugin")
	}

	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	data, err := os.ReadFile(filepath.Join(profile, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("package.json: %v", err)
	}
	if pkg.Dependencies[Name] == "" {
		t.Errorf("package.json lacks the dependency: %s", data)
	}

	patch, err := os.ReadFile(filepath.Join(profile, "cordis.patch.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch), "id: "+Name) {
		t.Errorf("patch lacks the insert row:\n%s", patch)
	}
}

// TestInstallIsIdempotent keeps a second startup from duplicating the row or
// rewriting the profile.
func TestInstallIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := Install(home); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "profiles", "web", "cordis.patch.yml")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(home); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("second install changed the patch:\n%s\n---\n%s", first, second)
	}
	if n := strings.Count(string(second), "id: "+Name); n != 1 {
		t.Errorf("patch has the row %d times:\n%s", n, second)
	}
}

// TestInstallKeepsTheProfile keeps the rows the user already had.
func TestInstallKeepsTheProfile(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profiles", "web")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "- id: llm-pi-ai\n  name: \"@deepseek-ai/dsh-llm-pi-ai\"\n"
	if err := os.WriteFile(filepath.Join(profile, "cordis.patch.yml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(home); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(profile, "cordis.patch.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), existing) {
		t.Errorf("the user's rows moved:\n%s", data)
	}
	if !strings.Contains(string(data), Name) {
		t.Errorf("the row was not added:\n%s", data)
	}
}

func TestEnabled(t *testing.T) {
	tests := map[string]bool{"": true, "1": true, "on": true, "0": false, "off": false, "no": false}
	for value, want := range tests {
		t.Setenv("DSH_GTK_DRAWER", value)
		if got := Enabled(); got != want {
			t.Errorf("Enabled() with %q = %v; want %v", value, got, want)
		}
	}
}
