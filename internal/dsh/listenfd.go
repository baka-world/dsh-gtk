package dsh

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

// listenfdScript adopts a listening socket the shell bound; see listenfd.js.
//
//go:embed listenfd.js
var listenfdScript []byte

// WriteShim writes the preload script under dir and returns its path, so it
// can be passed to node as --require.
func WriteShim(dir string) (string, error) {
	path := filepath.Join(dir, "listenfd.js")
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, listenfdScript) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, listenfdScript, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// isNode reports whether prog is a node interpreter: the preload only makes
// sense there, and a dsh that is an executable of its own binds its own port.
func isNode(prog string) bool {
	switch strings.ToLower(filepath.Base(prog)) {
	case "node", "node.exe":
		return true
	}
	return false
}
