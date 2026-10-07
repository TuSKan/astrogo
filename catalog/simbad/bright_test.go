package simbad

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// With no limit the bright query asks for every object, one row past the cap
// so that a cut list can be told from a complete one; with a limit, that
// many. Either way it leaves out objects with no position. It used to take
// TOP 100 for every request without a limit, which is how VisibleTonight saw
// SIMBAD's 100 brightest objects whatever its bound (#603).
func TestBuildBrightQueryAsksForEveryObject(t *testing.T) {
	t.Parallel()

	all := BuildBrightQuery(resolve.BrightRequest{MaxVMag: 6})
	if !strings.Contains(all, fmt.Sprintf("TOP %d", brightRowCap+1)) {
		t.Errorf("a request with no limit should ask for %d rows, got: %s", brightRowCap+1, all)
	}

	limited := BuildBrightQuery(resolve.BrightRequest{MaxVMag: 6, Limit: 50})
	if !strings.Contains(limited, "TOP 50\n") {
		t.Errorf("a request with a limit should ask for that many, got: %s", limited)
	}

	for _, q := range []string{all, limited} {
		if !strings.Contains(q, "basic.ra IS NOT NULL AND basic.dec IS NOT NULL") {
			t.Errorf("the bright query should leave out objects with no position, got: %s", q)
		}
	}
}

// brightServer serves rows synthetic objects in the bright query's CSV shape
// and records each request's MAXREC.
func brightServer(t *testing.T, rows int) (hits *atomic.Int32, maxrec *atomic.Value) {
	t.Helper()

	var b strings.Builder

	b.WriteString("oid,main_id,ra,dec,otype,pmra,pmdec,plx_value,rvz_radvel,vmag\n")

	for i := range rows {
		fmt.Fprintf(&b, "%d,* s%d,10.0,20.0,*,0,0,0,0,%.6f\n", i+1, i+1, float64(i)/float64(rows))
	}

	body := b.String()
	hits = new(atomic.Int32)
	maxrec = new(atomic.Value)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		maxrec.Store(r.PostForm.Get("MAXREC"))
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	redirect(t, remote.SIMBAD, server.URL)

	return hits, maxrec
}

// drainBright collects a SearchBright call: its targets and the errors it
// yielded.
func drainBright(p *Provider, req resolve.BrightRequest) (targets []resolve.Target, errs []error) {
	p.SearchBright(context.Background(), req)(func(tgt resolve.Target, err error) bool {
		if err != nil {
			errs = append(errs, err)
			return true
		}

		targets = append(targets, tgt)

		return true
	})

	return targets, errs
}

// Below the cap, a request with no limit gets every object, asks the service
// for one row past the cap (the service otherwise stops at its default 50,000
// without saying so), and the complete list is cached.
func TestSearchBrightReturnsEveryObjectBelowTheCap(t *testing.T) {
	hits, maxrec := brightServer(t, 3)

	p := New()

	targets, errs := drainBright(p, resolve.BrightRequest{MaxVMag: 6})
	if len(errs) != 0 || len(targets) != 3 {
		t.Fatalf("got %d targets and errors %v, want all 3 and none", len(targets), errs)
	}

	if got := maxrec.Load(); got != fmt.Sprint(brightRowCap+1) {
		t.Errorf("MAXREC = %v, want %d", got, brightRowCap+1)
	}

	if again, _ := drainBright(p, resolve.BrightRequest{MaxVMag: 6}); len(again) != 3 || hits.Load() != 1 {
		t.Errorf("a second call returned %d targets after %d requests; a complete list is cached",
			len(again), hits.Load())
	}
}

// Past the cap, the caller gets the brightest brightRowCap objects and then an
// error saying the list was cut, never a short list that looks complete, and
// the cut list is not cached.
func TestSearchBrightReportsACutList(t *testing.T) {
	hits, _ := brightServer(t, brightRowCap+1)

	p := New()

	targets, errs := drainBright(p, resolve.BrightRequest{MaxVMag: 9})
	if len(targets) != brightRowCap {
		t.Errorf("got %d targets, want the %d brightest", len(targets), brightRowCap)
	}

	if len(errs) != 1 || !errors.Is(errs[0], ErrBrightTruncated) {
		t.Fatalf("errors = %v, want one wrapping ErrBrightTruncated", errs)
	}

	if _, errs := drainBright(p, resolve.BrightRequest{MaxVMag: 9}); len(errs) != 1 || hits.Load() != 2 {
		t.Errorf("a second call made %d requests in all and yielded %v; a cut list must not be cached",
			hits.Load(), errs)
	}
}

// A request that sets its own limit gets that many, and no MAXREC: the cap is
// only for a request that asked for everything.
func TestSearchBrightHonorsAnExplicitLimit(t *testing.T) {
	_, maxrec := brightServer(t, 3)

	targets, errs := drainBright(New(), resolve.BrightRequest{MaxVMag: 6, Limit: 50})
	if len(errs) != 0 || len(targets) != 3 {
		t.Fatalf("got %d targets and errors %v", len(targets), errs)
	}

	if got := maxrec.Load(); got != "" {
		t.Errorf("MAXREC = %v on a request with its own limit, want none", got)
	}
}
