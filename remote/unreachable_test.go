package remote

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// silentHost is a TCP listener that accepts every connection and never
// answers: a host that is up enough to connect to and gives no answer, as
// SIMBAD's did during the CDS outage of 2026-11-13. It counts the
// connections, which is how a test tells a request sent from one refused
// before it left.
func silentHost(t *testing.T) (base string, accepted *atomic.Int32) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	accepted = new(atomic.Int32)

	var (
		mu    sync.Mutex
		conns []net.Conn
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

	return "http://" + ln.Addr().String() + "/sync", accepted
}

// silentClient is a client whose SIMBAD endpoint is base, with a timeout short
// enough to wait out in a test.
func silentClient(t *testing.T, base string) *Client {
	t.Helper()

	c := NewClient()
	if err := c.SetURL(SIMBAD, base); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	c.SetAPIOptions(WithTimeout(300 * time.Millisecond))

	return c
}

// TestAHostThatGivesNoAnswerIsAskedOnce: during the CDS outage, example 05's
// search waited 73 s for SIMBAD and its resolve, the next call, waited 61 s
// more. A client now remembers a host that gave no answer: the first request
// waits out its timeout and fails with ErrUnreachable, and the next fails at
// once, without connecting, citing the first. A mirror set with SetURL is
// another host, and is asked.
func TestAHostThatGivesNoAnswerIsAskedOnce(t *testing.T) {
	t.Parallel()

	base, accepted := silentHost(t)
	c := silentClient(t, base)
	form := url.Values{"QUERY": {"SELECT 1"}}

	_, first := c.PostForm(t.Context(), SIMBAD, "", form)
	if !errors.Is(first, ErrUnreachable) {
		t.Fatalf("first request: err = %v, want ErrUnreachable", first)
	}

	asked := accepted.Load()
	if asked == 0 {
		t.Fatal("the first request never connected")
	}

	start := time.Now()
	_, second := c.PostForm(t.Context(), SIMBAD, "", form)

	if !errors.Is(second, ErrUnreachable) || !strings.Contains(second.Error(), "gave no answer") {
		t.Errorf("second request: err = %v, want ErrUnreachable citing the first", second)
	}

	if took := time.Since(start); took > 100*time.Millisecond {
		t.Errorf("second request took %v, want an immediate refusal", took)
	}

	if n := accepted.Load(); n != asked {
		t.Errorf("second request connected %d more times, want none", n-asked)
	}

	// GetJSON goes through the same memory.
	var out struct{}
	if err := c.GetJSON(t.Context(), SIMBAD, "", nil, &out); !errors.Is(err, ErrUnreachable) {
		t.Errorf("GetJSON: err = %v, want ErrUnreachable", err)
	}

	mirror, mirrorAccepted := silentHost(t)
	if err := c.SetURL(SIMBAD, mirror); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	if _, err := c.PostForm(t.Context(), SIMBAD, "", form); !errors.Is(err, ErrUnreachable) || mirrorAccepted.Load() == 0 {
		t.Errorf("a mirror was not asked: err = %v, connections %d", err, mirrorAccepted.Load())
	}
}

// TestAnAnswerIsNeverNoAnswer: a host that answers with a failing status
// answered. It is an HTTPError, not ErrUnreachable, and the next request asks
// again; and an answer clears a host the client remembered.
func TestAnAnswerIsNeverNoAnswer(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "not here", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	c := silentClient(t, srv.URL)

	for i := range 2 {
		_, err := c.Get(t.Context(), SIMBAD, "", nil)

		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || errors.Is(err, ErrUnreachable) {
			t.Fatalf("request %d: err = %v, want an HTTPError and not ErrUnreachable", i, err)
		}
	}

	if n := hits.Load(); n != 2 {
		t.Errorf("the server was asked %d times, want 2", n)
	}

	// An answer clears what was remembered, as when one request gets an
	// answer while another, sent beside it, got none.
	host := hostOf(srv.URL)
	c.noAnswerMu.Lock()
	c.noAnswer[host] = noAnswer{at: time.Now(), err: errors.New("earlier")}
	c.noAnswerMu.Unlock()

	if err := c.heard(t.Context(), host, &HTTPError{StatusCode: http.StatusNotFound}); errors.Is(err, ErrUnreachable) {
		t.Errorf("heard wrapped an answer as no answer: %v", err)
	}

	if err := c.answering(host); err != nil {
		t.Errorf("an answer did not clear the host: %v", err)
	}
}

// TestTheCallersOwnCancellationIsNotTheHosts: a request the caller's context
// ended says nothing about the host, so nothing is remembered and the next
// request is sent.
func TestTheCallersOwnCancellationIsNotTheHosts(t *testing.T) {
	t.Parallel()

	base, accepted := silentHost(t)
	c := silentClient(t, base)
	c.SetAPIOptions(WithTimeout(5 * time.Second))

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	if _, err := c.Get(ctx, SIMBAD, "", nil); err == nil || errors.Is(err, ErrUnreachable) {
		t.Fatalf("a request the caller cancelled: err = %v, want the cancellation and not ErrUnreachable", err)
	}

	asked := accepted.Load()

	ctx2, cancel2 := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel2()

	_, _ = c.Get(ctx2, SIMBAD, "", nil)

	if accepted.Load() == asked {
		t.Error("the next request was not sent: the caller's own deadline was remembered against the host")
	}
}

// TestTheCooldownEnds: with no cooldown nothing is remembered, and after a
// cooldown the host is asked again. A clone keeps the cooldown, which is
// policy, and not the memory, which is state.
func TestTheCooldownEnds(t *testing.T) {
	t.Parallel()

	base, accepted := silentHost(t)

	forgetful := silentClient(t, base)
	forgetful.SetUnreachableCooldown(0)

	for range 2 {
		_, _ = forgetful.Get(t.Context(), SIMBAD, "", nil)
	}

	if n := accepted.Load(); n < 2 {
		t.Errorf("with no cooldown, two requests connected %d times, want both sent", n)
	}

	brief := silentClient(t, base)
	brief.SetUnreachableCooldown(400 * time.Millisecond)

	_, _ = brief.Get(t.Context(), SIMBAD, "", nil)
	before := accepted.Load()

	if _, err := brief.Get(t.Context(), SIMBAD, "", nil); !errors.Is(err, ErrUnreachable) || accepted.Load() != before {
		t.Fatalf("inside the cooldown: err = %v, %d more connections", err, accepted.Load()-before)
	}

	clone := brief.Clone()
	if clone.cooldown != brief.cooldown {
		t.Errorf("clone's cooldown %v, want its parent's %v", clone.cooldown, brief.cooldown)
	}

	_, _ = clone.Get(t.Context(), SIMBAD, "", nil)

	if accepted.Load() == before {
		t.Error("a clone refused a host only its parent remembered")
	}

	time.Sleep(500 * time.Millisecond)

	after := accepted.Load()
	_, _ = brief.Get(t.Context(), SIMBAD, "", nil)

	if accepted.Load() == after {
		t.Error("after the cooldown the host was not asked again")
	}
}
