package sbdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// manyBodies serves the SBDB Query API with more asteroids than the default
// limit and a few comets, honoring the request's limit as SBDB does, and
// records the last limit asked for and how many requests arrived.
func manyBodies(t *testing.T, asteroids, comets int, lastLimit *atomic.Int64, requests *atomic.Int32) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			http.Error(w, "no limit", http.StatusBadRequest)
			return
		}

		lastLimit.Store(int64(limit))

		n, field := asteroids, "H"
		if r.URL.Query().Get("sb-kind") == "c" {
			n, field = comets, "M1"
		}

		rows := make([]string, 0, min(n, limit))
		for i := range min(n, limit) {
			rows = append(rows, fmt.Sprintf(`["%d Body", "%d", %g, 0.15, "MBA", 0.1]`, i+1, 20000000+i, 3+float64(i)/100))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"signature":{"version":"1.0"},"fields":["full_name","spkid","%s","G","class","e"],"data":[%s],"count":%d}`,
			field, strings.Join(rows, ","), len(rows))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.JPLSBDBQuery, server.URL); err != nil {
		t.Fatal(err)
	}
}

// TestSearchBrightSaysWhenItIsCut: with no limit of the caller's own, a kind
// with more candidates than the default of 50 gives its 50 brightest and then
// an error wrapping resolve.ErrTruncated, and the cut list is not cached
// (#776). It used to stop at 50 silently, a list that looked complete.
func TestSearchBrightSaysWhenItIsCut(t *testing.T) { //nolint:paralleltest // remote.SetURL is process-wide
	var (
		lastLimit atomic.Int64
		requests  atomic.Int32
	)

	manyBodies(t, 120, 3, &lastLimit, &requests)

	prov := New()

	for pass := range 2 {
		var (
			got    []resolve.Target
			errSaw error
		)

		prov.SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 9})(func(tgt resolve.Target, err error) bool {
			if err != nil {
				errSaw = err

				return true
			}

			got = append(got, tgt)

			return true
		})

		if len(got) != 53 {
			t.Errorf("pass %d: %d targets, want the 50 brightest asteroids and all 3 comets", pass, len(got))
		}

		if !errors.Is(errSaw, resolve.ErrTruncated) {
			t.Errorf("pass %d: error %v, want ErrTruncated after a list cut at the default", pass, errSaw)
		}
	}

	if lastLimit.Load() != 51 {
		t.Errorf("asked for %d rows per kind, want one past the default of 50", lastLimit.Load())
	}

	if n := requests.Load(); n != 4 {
		t.Errorf("%d requests over two calls, want 4: a cut list must not be served from the cache", n)
	}
}

// TestSearchBrightKeepsTheCallersLimit: a limit the caller set is what they
// asked for, so a list that stops there is complete for them.
func TestSearchBrightKeepsTheCallersLimit(t *testing.T) { //nolint:paralleltest // remote.SetURL is process-wide
	var (
		lastLimit atomic.Int64
		requests  atomic.Int32
	)

	manyBodies(t, 120, 3, &lastLimit, &requests)

	var (
		got    []resolve.Target
		errSaw error
	)

	New().SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 9, Limit: 5})(func(tgt resolve.Target, err error) bool {
		if err != nil {
			errSaw = err

			return true
		}

		got = append(got, tgt)

		return true
	})

	if errSaw != nil || len(got) != 8 {
		t.Errorf("got %d targets and %v, want 5 asteroids, 3 comets and no error", len(got), errSaw)
	}

	if lastLimit.Load() != 5 {
		t.Errorf("asked for %d rows per kind, want the caller's 5", lastLimit.Load())
	}
}
