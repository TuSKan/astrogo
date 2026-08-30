package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/time"
)

// A paced client must actually wait, and must not let concurrent callers
// bypass the wait by racing.
//
// This is a politeness policy rather than a correctness one, which is exactly
// why it needs a test: nothing downstream fails when it stops working. The
// service being protected just starts refusing to answer, hours later, in a
// way that looks like an outage.
func TestMinIntervalPacesAndSerialises(t *testing.T) {
	var (
		mu     sync.Mutex
		starts []time.GoTime
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()

		starts = append(starts, time.Now())

		mu.Unlock()

		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	const id = remote.SIMBAD

	redirect(t, id, srv.URL+"/")

	const gap = 120 * time.Millisecond

	client := newClient(t, id, api.WithMinInterval(gap))

	// Four requests fired at once: the pacing has to hold them apart even
	// though nothing else does.
	var wg sync.WaitGroup

	for range 4 {
		wg.Go(func() {
			r, err := client.Get(context.Background(), id, "", nil)
			if err != nil {
				t.Errorf("Get: %v", err)

				return
			}

			_ = r.Close()
		})
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(starts) != 4 {
		t.Fatalf("server saw %d requests, want 4", len(starts))
	}

	for i := 1; i < len(starts); i++ {
		if d := starts[i].Sub(starts[i-1]); d < gap/2 {
			t.Errorf("requests %d and %d were %v apart, want at least %v", i-1, i, d, gap)
		}
	}
}

// An unpaced client keeps its old behaviour and pays nothing.
func TestWithoutMinIntervalThereIsNoDelay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	const id = remote.SIMBAD

	redirect(t, id, srv.URL+"/")

	client := newClient(t, id)

	start := time.Now()

	for range 5 {
		r, err := client.Get(context.Background(), id, "", nil)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		_ = r.Close()
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("five unpaced requests took %v; pacing must be opt-in", elapsed)
	}
}

// A caller that gives up while waiting for its slot gets its context error,
// not a request it did not want.
func TestPacingRespectsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	const id = remote.SIMBAD

	redirect(t, id, srv.URL+"/")

	client := newClient(t, id, api.WithMinInterval(10*time.Second))

	// The first request sets the clock; the second must wait ten seconds.
	r, err := client.Get(context.Background(), id, "", nil)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}

	_ = r.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := client.Get(ctx, id, "", nil); err == nil {
		t.Error("a cancelled wait must not proceed to the request")
	}
}
