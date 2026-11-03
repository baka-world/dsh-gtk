//go:build !linux || !(amd64 || arm64)

package gtk

// Style is a no-op where the shell is not GTK: nothing here can restyle it.
func Style(string) error { return ErrUnsupported }
