package api_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote/api"
)

// connServer is an httptest server that counts the connections it has seen
// opened and closed. The server is the only place a released connection can
// be seen from outside the client: a connection the client merely forgets is
// still open there.
type connServer struct {
	*httptest.Server

	opened, closed atomic.Int32
}

func newConnServer(t *testing.T) *connServer {
	t.Helper()

	s := &connServer{}
	s.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			s.opened.Add(1)
		}

		if state == http.StateClosed {
			s.closed.Add(1)
		}
	}

	s.Start()
	t.Cleanup(s.Close)

	return s
}

// closedWithin returns how many connections the server has seen closed, once
// it has seen want or the wait is over. A client closing a connection reaches
// the server asynchronously, so the count is polled rather than read once.
// The wait is far longer than the milliseconds that takes, and it bounds only
// a failure.
func (s *connServer) closedWithin(want int32, wait time.Duration) int32 {
	timeout := time.NewTimer(wait)
	defer timeout.Stop()

	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for s.closed.Load() < want {
		select {
		case <-timeout.C:
			return s.closed.Load()
		case <-tick.C:
		}
	}

	return s.closed.Load()
}

// TestCloseReleasesTheIdleConnections holds Close to its doc: a connection a
// finished request left idle is closed, not kept until the transport's idle
// timeout. resty's own Close never reaches the transport (#1089).
func TestCloseReleasesTheIdleConnections(t *testing.T) {
	srv := newConnServer(t)
	c := api.NewClient(0)

	body, err := c.Get(context.Background(), srv.URL, "", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()

	if got := srv.closedWithin(1, 200*time.Millisecond); got != 0 {
		t.Fatalf("fixture: %d connections closed before Close; the request should have left one idle", got)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := srv.closedWithin(1, 5*time.Second); got != 1 {
		t.Errorf("after Close the server saw %d of its %d connections closed, want the idle one closed", got, srv.opened.Load())
	}
}
