package eop

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// TestEOPFetchWithoutConsentAsksNoSource: the lazy load's network step is
// refused before any request when consent is absent, even with a bulletin
// cached (#964). That bulletin is what Cached reads, and it still does. Before
// this, Fetch asked the IERS source whether the cache was current, and a
// lookup past the bulletin sat out 17 s against a source that did not answer.
func TestEOPFetchWithoutConsentAsksNoSource(t *testing.T) {
	scratchCache(t)

	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.ServeContent(w, r, "finals2000A.all", time.Unix(1_700_000_000, 0), strings.NewReader(sampleFinals2000A))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatal(err)
	}

	if err := remote.WriteFile(ctx, fsys, prefix+eopCacheName, strings.NewReader(sampleFinals2000A)); err != nil {
		t.Fatalf("seed the cache: %v", err)
	}

	if _, err := (eopLoader{}).Fetch(ctx); !errors.Is(err, remote.ErrDownloadDenied) {
		t.Errorf("Fetch without consent, a bulletin cached = %v, want ErrDownloadDenied", err)
	}

	if n := requests.Load(); n != 0 {
		t.Errorf("the IERS source was sent %d requests without consent, want 0", n)
	}

	data, err := (eopLoader{}).Cached(ctx)
	if err != nil || len(data.Raw) == 0 {
		t.Errorf("Cached without consent = %d bytes, %v: want the cached bulletin", len(data.Raw), err)
	}
}
