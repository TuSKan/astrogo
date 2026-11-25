package openngc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// TestResolveReadsTheCachedCatalogWhenItsSourceErrs is #1279's symptom: with
// the catalog cached and its source answering every request with an error,
// as a machine with no network sees it, a name still resolves. It used to fail
// with "catalog unavailable" unless the program had turned offline mode on.
func TestResolveReadsTheCachedCatalogWhenItsSourceErrs(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	// A first provider fills the cache from a working source.
	fakeSources(t)
	loadNow(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.OpenNGC, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	// A second provider holds nothing in memory, so it reads the catalog anew.
	got, err := New().Resolve(context.Background(), "M42")
	if err != nil {
		t.Fatalf("Resolve(M42) with the source failing and the catalog cached: %v", err)
	}

	if got.ID != "NGC1976" {
		t.Errorf("Resolve(M42) = %q, want NGC1976", got.ID)
	}
}
