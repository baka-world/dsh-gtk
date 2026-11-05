package main

import (
	"net"
	"strconv"
	"testing"
)

// listen takes a port and returns it, so a test can hand bindPort a port that
// is certainly owned.
func listen(t *testing.T) (port int, release func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l.Addr().(*net.TCPAddr).Port, func() { _ = l.Close() }
}

// TestBindPortSkipsATakenPort pins the fix for a preferred port that another
// dsh owns for good: the app used to find out by booting, which costs a failed
// start and a random origin for the web UI.
func TestBindPortSkipsATakenPort(t *testing.T) {
	busy, release := listen(t)
	defer release()

	ln, got := bindPort(busy, 10)
	if ln == nil {
		t.Fatalf("bindPort(%d) found no port", busy)
	}
	defer ln.Close()
	if got == busy {
		t.Fatalf("bindPort(%d) returned the taken port", busy)
	}
	if got != busy+1 {
		t.Errorf("bindPort(%d) = %d; want the next port, %d", busy, got, busy+1)
	}
	if addr := ln.Addr().(*net.TCPAddr); addr.Port != got {
		t.Errorf("listener is on %d, not %d", addr.Port, got)
	}
	if _, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(got))); err == nil {
		t.Errorf("bindPort returned %d but it was not bound", got)
	}
}

func TestBindPortKeepsAFreePort(t *testing.T) {
	port, release := listen(t)
	release() // free again; nothing else is likely to take it in between
	ln, got := bindPort(port, 3)
	if ln == nil {
		t.Fatal("bindPort found no port")
	}
	defer ln.Close()
	if got != port {
		t.Errorf("bindPort(%d) = %d; want the port itself", port, got)
	}
}

func TestBindPortGivesUp(t *testing.T) {
	port, release := listen(t)
	defer release()
	if ln, got := bindPort(port, 1); ln != nil || got != 0 {
		t.Errorf("bindPort(%d, 1) = %v, %d; want nil so the OS picks a port", port, ln, got)
	}
}
