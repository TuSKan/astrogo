package iers

import (
	"context"
	"errors"
	"testing"
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

// TestLoadWithNoLoaderSaysSo: with no loader registered Load is ErrNoLoader.
func TestLoadWithNoLoaderSaysSo(t *testing.T) {
	installLoader(t, nil)

	if err := Load(context.Background()); !errors.Is(err, ErrNoLoader) {
		t.Errorf("Load with no loader = %v, want ErrNoLoader", err)
	}
}
