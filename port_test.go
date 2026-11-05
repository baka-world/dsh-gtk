package main

import (
	"net"
	"strconv"
	"testing"
)

// listen takes a port and returns it, so a test can hand freePort a port that
// is certainly owned.
func listen(t *testing.T) (port int, close func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l.Addr().(*net.TCPAddr).Port, func() { _ = l.Close() }
}

// TestFreePortSkipsATakenPort pins the fix for a preferred port that another
// dsh owns for good: the app used to find out by booting, which costs a failed
// start and a random origin for the web UI.
func TestFreePortSkipsATakenPort(t *testing.T) {
	busy, release := listen(t)
	defer release()

	got := freePort(busy, 10)
	if got == busy {
		t.Fatalf("freePort(%d) returned the taken port", busy)
	}
	if got != busy+1 {
		t.Errorf("freePort(%d) = %d; want the next port, %d", busy, got, busy+1)
	}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(got)))
	if err != nil {
		t.Fatalf("freePort returned %d, which cannot be listened on: %v", got, err)
	}
	_ = l.Close()
}

func TestFreePortKeepsAFreePort(t *testing.T) {
	port, release := listen(t)
	release() // free again; nothing else is likely to take it in between
	if got := freePort(port, 3); got != port {
		t.Errorf("freePort(%d) = %d; want the port itself", port, got)
	}
}

func TestFreePortGivesUp(t *testing.T) {
	port, release := listen(t)
	defer release()
	if got := freePort(port, 1); got != port {
		t.Errorf("freePort(%d, 1) = %d; want the port back so the boot reports it", port, got)
	}
}
