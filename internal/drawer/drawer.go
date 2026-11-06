// Package drawer ships the dsh-gtk drawer as a dsh client plugin: the sidebar
// becomes an overlay that slides over the transcript instead of squeezing it.
//
// The plugin is not a fork of dsh. A package whose package.json declares
// dsh.client is composed into the page's plugin list at startup and served from
// /plugins/<name>/client.js (see the __DSH_BOOT__ entries in the served HTML),
// so the whole change is a package in the profile's node_modules plus one row
// in the profile's patch layer.
//
// Why a drawer: the stock layout animates the grid's sidebar track, which
// changes the transcript's width on every frame and makes the renderer reflow
// every rendered message. Measured on a 3.3k-node conversation, that cost 14–20
// fps with 200–350 ms frames; with the sidebar out of the grid the same toggle
// runs at 54–58 fps with 40–50 ms frames, the same as removing the transcript
// from the DOM altogether.
package drawer

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Name is the package name and the plugin row's id.
const Name = "dsh-gtk-drawer"

//go:embed files
var files embed.FS

// Enabled reports whether the plugin is installed into the profile at startup.
// DSH_GTK_DRAWER=0 leaves the stock layout alone.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSH_GTK_DRAWER"))) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

// Install makes the profile under home load the drawer plugin. It is
// idempotent: files are written only when their content differs, and the two
// profile edits are applied once.
func Install(home string) error {
	if home == "" {
		return errors.New("drawer: no dsh home")
	}
	profile := filepath.Join(home, "profiles", "web")
	dir := filepath.Join(profile, "node_modules", Name)

	err := fs.WalkDir(files, "files", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, strings.TrimPrefix(path, "files/"))
		if old, err := os.ReadFile(target); err == nil && string(old) == string(data) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return fmt.Errorf("drawer: write plugin: %w", err)
	}
	if err := ensureDependency(filepath.Join(profile, "package.json")); err != nil {
		return err
	}
	return ensurePatch(filepath.Join(profile, "cordis.patch.yml"))
}

// ensureDependency lists the plugin in the profile's dependencies, which is
// what makes the loader find it in node_modules.
func ensureDependency(path string) error {
	var pkg map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &pkg); err != nil {
			return fmt.Errorf("drawer: %s: %w", path, err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// A fresh home: the profile needs the bundles a web profile is made of,
		// or dsh has nothing to serve.
		pkg = map[string]any{
			"name":    "dsh-profile-web",
			"private": true,
			"dsh": map[string]any{
				"profile": map[string]any{
					"bundles": []string{"@deepseek-ai/dsh-base", "@deepseek-ai/dsh-web-app"},
				},
			},
		}
	} else {
		return fmt.Errorf("drawer: %s: %w", path, err)
	}
	deps, _ := pkg["dependencies"].(map[string]any)
	if deps == nil {
		deps = map[string]any{}
	}
	if v, ok := deps[Name].(string); ok && v == "0.1.0" {
		return nil
	}
	deps[Name] = "0.1.0"
	pkg["dependencies"] = deps
	out, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// ensurePatch adds the insert row to the profile's patch layer, the file dsh
// reads after the bundles.
func ensurePatch(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("drawer: %s: %w", path, err)
	}
	if strings.Contains(string(data), Name) {
		return nil
	}
	row := "- insert:\n    - id: " + Name + "\n      name: '" + Name + "'\n"
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text+row), 0o644)
}
