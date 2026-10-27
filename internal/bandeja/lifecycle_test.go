package bandeja_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/bandeja"
)

func waitReturns(t *testing.T, srv *bandeja.Server) <-chan error {
	t.Helper()
	ch := make(chan error, 1)
	go func() { ch <- srv.Wait() }()
	return ch
}

// A freshly started server must not be treated as idle: lastReq starts at
// Start time, not at the zero time.
func TestLifecycle_NotIdleRightAfterStart(t *testing.T) {
	defer bandeja.SetIdleForTest(10*time.Millisecond, time.Hour)()
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	time.Sleep(150 * time.Millisecond)
	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatalf("server closed right after start: %v", err)
	}
	resp.Body.Close()
}

func TestLifecycle_WaitReturnsAfterIdleShutdown(t *testing.T) {
	defer bandeja.SetIdleForTest(10*time.Millisecond, 50*time.Millisecond)()
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	srv, err := bandeja.Start(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	select {
	case err := <-waitReturns(t, srv):
		if err != nil {
			t.Fatalf("Wait after idle = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after idle shutdown")
	}
}

func TestLifecycle_WaitReturnsOnContextCancelAndClose(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	w := waitReturns(t, srv)
	cancel()
	select {
	case <-w:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after ctx cancel")
	}

	srv2, err := bandeja.Start(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	w2 := waitReturns(t, srv2)
	srv2.Close()
	select {
	case <-w2:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after Close")
	}
}

type failingListener struct{ net.Listener }

func (failingListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }

func TestLifecycle_ServeErrorIsReturnedByWait(t *testing.T) {
	defer bandeja.SetListenForTest(func(n, a string) (net.Listener, error) {
		ln, err := net.Listen(n, a)
		return failingListener{ln}, err
	})()
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	srv, err := bandeja.Start(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	select {
	case err := <-waitReturns(t, srv):
		if err == nil {
			t.Fatal("Wait = nil, want the serve error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after serve error")
	}
}

func TestLifecycle_StartWithIdleUsesGivenLimit(t *testing.T) {
	defer bandeja.SetIdleForTest(10*time.Millisecond, time.Hour)()
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	srv, err := bandeja.StartWithIdle(context.Background(), s, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	select {
	case <-waitReturns(t, srv):
	case <-time.After(2 * time.Second):
		t.Fatal("explicit idle limit was ignored")
	}
}

func TestLifecycle_DefaultIdleIsTenMinutes(t *testing.T) {
	if bandeja.DefaultIdle != 10*time.Minute {
		t.Fatalf("DefaultIdle = %v, want 10m", bandeja.DefaultIdle)
	}
}
