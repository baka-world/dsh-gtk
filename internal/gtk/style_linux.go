//go:build linux && (amd64 || arm64)

package gtk

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// GTK's priorities: the theme's default, an application's, and the user's,
// which wins over both.
const priorityUser = 800

// gerror is a GError: GQuark domain; gint code; gchar *message.
type gerror struct {
	domain  uint32
	code    int32
	message *byte
}

// text formats the error the way GTK's messages read, or "" without one.
func (e *gerror) text() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(" (%d %d: %s)", e.domain, e.code, goString(e.message))
}

func goString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

type api struct {
	gtk, gdk uintptr

	addProviderForScreen func(screen, provider uintptr, priority uint32)
	providerNew          func() uintptr
	loadFromData         func(provider uintptr, data *byte, length int, err **gerror) bool
	screenGetDefault     func() uintptr
}

var (
	once  sync.Once
	bound *api
	err   error

	// provider is the app's style provider, kept for the app's life so that
	// a later Style call replaces its sheet instead of stacking another.
	provider uintptr
)

func load() (*api, error) {
	once.Do(func() {
		a := &api{}
		if a.gtk, err = purego.Dlopen("libgtk-3.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
			return
		}
		if a.gdk, err = purego.Dlopen("libgdk-3.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
			return
		}
		purego.RegisterLibFunc(&a.addProviderForScreen, a.gtk, "gtk_style_context_add_provider_for_screen")
		purego.RegisterLibFunc(&a.providerNew, a.gtk, "gtk_css_provider_new")
		purego.RegisterLibFunc(&a.loadFromData, a.gtk, "gtk_css_provider_load_from_data")
		purego.RegisterLibFunc(&a.screenGetDefault, a.gdk, "gdk_screen_get_default")
		bound = a
	})
	return bound, err
}

// Style adds css to every GTK widget of the app, at the user's priority. It
// must run on the GTK main thread, once the toolkit is up: the app's
// WhenReady callback is.
func Style(css string) error {
	if css == "" {
		return nil
	}
	a, err := load()
	if err != nil {
		return fmt.Errorf("gtk: load the toolkit: %w", err)
	}
	screen := a.screenGetDefault()
	if screen == 0 {
		return errors.New("gtk: no screen")
	}
	if provider == 0 {
		provider = a.providerNew()
		if provider == 0 {
			return errors.New("gtk: create a style provider")
		}
		a.addProviderForScreen(screen, provider, priorityUser)
	}
	// GTK 3 takes the length, and reports a bad sheet instead of aborting.
	data := []byte(css)
	var gerr *gerror
	if ok := a.loadFromData(provider, &data[0], len(data), &gerr); !ok {
		return fmt.Errorf("gtk: invalid CSS%s", gerr.text())
	}
	return nil
}
