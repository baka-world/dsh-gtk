//go:build linux && (amd64 || arm64)

package gtk

import "testing"

func TestGerrorText(t *testing.T) {
	msg := append([]byte("invalid property"), 0)
	e := &gerror{domain: 4, code: 7, message: &msg[0]}
	if got, want := e.text(), " (4 7: invalid property)"; got != want {
		t.Errorf("text() = %q; want %q", got, want)
	}
}

func TestGerrorNil(t *testing.T) {
	var e *gerror
	if got := e.text(); got != "" {
		t.Errorf("text() of no error = %q; want empty", got)
	}
}

func TestGoString(t *testing.T) {
	msg := append([]byte("hello"), 0)
	if got := goString(&msg[0]); got != "hello" {
		t.Errorf("goString() = %q", got)
	}
	if got := goString(nil); got != "" {
		t.Errorf("goString(nil) = %q", got)
	}
}
