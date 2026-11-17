package api

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// TestTheTimeoutBoundsTheWholeCall: a host that accepts connections and
// never answers cost a call one timeout per attempt, four with the default
// retries: 73 s against SIMBAD during the CDS outage of 2026-11-13. The
// timeout now bounds the call, so it ends within one timeout however many
// retries the policy allows, and the failure says no answer came.
func TestTheTimeoutBoundsTheWholeCall(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var (
		accepted atomic.Int32
		mu       sync.Mutex
		conns    []net.Conn
	)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			accepted.Add(1)
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()

		mu.Lock()
		defer mu.Unlock()

		for _, c := range conns {
			_ = c.Close()
		}
	})

	const timeout = 400 * time.Millisecond

	c := NewClient(timeout, WithRetries(3), WithIdempotentPost())

	for _, call := range []struct {
		name string
		do   func() error
	}{
		{"Get", func() error {
			_, err := c.Get(t.Context(), "http://"+ln.Addr().String(), "/x", nil)

			return err
		}},
		{"PostForm", func() error {
			_, err := c.PostForm(t.Context(), "http://"+ln.Addr().String(), "/x", nil)

			return err
		}},
	} {
		start := time.Now()
		err := call.do()
		took := time.Since(start)

		if !errors.Is(err, ErrNoResponse) {
			t.Errorf("%s: err = %v, want ErrNoResponse", call.name, err)
		}

		// Four attempts of one timeout each would take 1.6 s and more.
		if took > timeout*3/2 {
			t.Errorf("%s took %v, want at most about one timeout (%v)", call.name, took, timeout)
		}
	}
}

// TestABodyIsReadAfterTheCallReturns: the budget bounds reading the body
// too, so it has to outlive the call that returned it. A body read after Get
// returns is whole, and closing it releases the budget.
func TestABodyIsReadAfterTheCallReturns(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("the whole answer"))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(time.Second)

	body, err := c.Get(t.Context(), srv.URL, "", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	got, err := io.ReadAll(body)
	if err != nil || string(got) != "the whole answer" {
		t.Errorf("read %q, %v after Get returned, want the whole answer", got, err)
	}

	if err := body.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestAStatusIsAnAnswer: a failing status is the service's answer, an
// *HTTPError, and never ErrNoResponse.
func TestAStatusIsAnAnswer(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no such object", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	_, err := NewClient(time.Second).Get(t.Context(), srv.URL, "", nil)

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || errors.Is(err, ErrNoResponse) {
		t.Errorf("err = %v, want an *HTTPError and not ErrNoResponse", err)
	}
}
