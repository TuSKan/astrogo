package iers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// errFetchDown is a loader's network step failing.
var errFetchDown = errors.New("the IERS is down")

// TestLoadTakesTheFetchThenTheCache holds Load, the step epoch.LoadEOP takes, to
// its order: the network step's bulletin when it gives one that parses, else
// the cached one, installed as the caller's own choice with its source; and
// with neither, both errors and the model untouched (#1187).
func TestLoadTakesTheFetchThenTheCache(t *testing.T) {
	for _, c := range []struct {
		name       string
		loader     *fakeLoader
		wantSource string
		wantErrs   []error
	}{
		{"the network answers", &fakeLoader{fetched: Data{Raw: []byte(sampleFinals2000A)}}, SourceNetwork, nil},
		{"the download refused, a bulletin cached",
			&fakeLoader{fetchErr: ErrNoEOPData, cached: Data{Raw: []byte(sampleFinals2000A)}}, SourceCache, nil},
		{"the network sends junk",
			&fakeLoader{fetched: Data{Raw: []byte("<html>not a bulletin</html>")}, cached: Data{Raw: []byte(sampleFinals2000A)}},
			SourceCache, nil},
		{"neither", &fakeLoader{fetchErr: errFetchDown, cachedErr: ErrNoEOPData}, SourceZero,
			[]error{errFetchDown, ErrNoEOPData}},
	} {
		t.Run(c.name, func(t *testing.T) {
			installLoader(t, c.loader)

			err := Load(t.Context())

			switch {
			case c.wantErrs == nil && err != nil:
				t.Errorf("Load = %v, want nil", err)
			case c.wantErrs != nil:
				for _, want := range c.wantErrs {
					if !errors.Is(err, want) {
						t.Errorf("Load = %v, want it to carry %v", err, want)
					}
				}
			}

			if got := EOPSource(); got != c.wantSource {
				t.Errorf("EOPSource = %q, want %q", got, c.wantSource)
			}

			if c.wantErrs == nil && !modelIsExplicit() {
				t.Error("a loaded bulletin is not marked the caller's choice; the lazy load would replace it")
			}
		})
	}
}

// TestLoadReadsThroughTheRemoteLoader: with no loader registered, Load asks
// remote. Offline the fetch is refused at the gate, and the bulletin
// pre-seeded in remote's cache is what Load installs.
func TestLoadReadsThroughTheRemoteLoader(t *testing.T) {
	installLoader(t, nil)
	t.Cleanup(remote.Capture().Restore)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	remote.SetOffline(true)

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	if err := remote.WriteFile(ctx, fsys, prefix+cacheName, strings.NewReader(sampleFinals2000A)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	if err := Load(ctx); err != nil {
		t.Fatalf("Load with a pre-seeded bulletin, offline: %v", err)
	}

	if got := EOPSource(); got != SourceCache {
		t.Errorf("EOPSource = %q, want %q", got, SourceCache)
	}
}
