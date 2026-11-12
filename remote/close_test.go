package remote_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote"
)

// countingServer is an httptest server that counts the connections it has seen
// opened and closed. A client that only forgets a transport leaves that
// transport's connections open, and the server is the one place that shows it.
type countingServer struct {
	*httptest.Server

	opened, closed atomic.Int32
}

func newCountingServer(t *testing.T) *countingServer {
	t.Helper()

	s := &countingServer{}
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
// it has seen want or the wait is over. A closed connection reaches the server
// asynchronously, so the count is polled rather than read once.
func (s *countingServer) closedWithin(want int32, wait time.Duration) int32 {
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

// twoEndpointClient builds a client with two API endpoints aimed at srv. Each
// endpoint gets a transport of its own, so one request to each leaves two idle
// connections that only that endpoint's transport can release.
func twoEndpointClient(t *testing.T, srv *countingServer) (*remote.Client, []remote.EndpointID) {
	t.Helper()

	c := remote.NewClient()
	ids := []remote.EndpointID{remote.ESOSkyCalc, remote.MAST}

	for _, id := range ids {
		if err := c.SetURL(id, srv.URL); err != nil {
			t.Fatalf("SetURL(%s): %v", id, err)
		}

		body, err := c.Get(context.Background(), id, "", nil)
		if err != nil {
			t.Fatalf("Get(%s): %v", id, err)
		}

		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}

	if opened := srv.opened.Load(); opened != int32(len(ids)) {
		t.Fatalf("fixture: %d connections opened for %d endpoints, want one each", opened, len(ids))
	}

	if closed := srv.closedWithin(1, 200*time.Millisecond); closed != 0 {
		t.Fatalf("fixture: %d connections closed before the call under test", closed)
	}

	return c, ids
}

// TestCloseReleasesEveryTransportsIdleConnections holds Client.Close to its
// doc. Until #1089 it closed nothing: it forgot its transports, and their
// connections stayed open until the transport's idle timeout.
func TestCloseReleasesEveryTransportsIdleConnections(t *testing.T) {
	srv := newCountingServer(t)
	c, ids := twoEndpointClient(t, srv)

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := srv.closedWithin(int32(len(ids)), 5*time.Second); got != int32(len(ids)) {
		t.Errorf("after Close the server saw %d of %d connections closed, want every one", got, len(ids))
	}
}

// TestSetAPIOptionsReleasesTheTransportsItDiscards: SetAPIOptions drops every
// transport so the next request opens one under the new settings. A dropped
// transport's connections are released, not left open behind it (#1089).
func TestSetAPIOptionsReleasesTheTransportsItDiscards(t *testing.T) {
	srv := newCountingServer(t)
	c, ids := twoEndpointClient(t, srv)

	t.Cleanup(func() { _ = c.Close() })

	c.SetAPIOptions(remote.WithTimeout(30 * time.Second))

	if got := srv.closedWithin(int32(len(ids)), 5*time.Second); got != int32(len(ids)) {
		t.Errorf("after SetAPIOptions the server saw %d of %d connections closed, want every one", got, len(ids))
	}
}
