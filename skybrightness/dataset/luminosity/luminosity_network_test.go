//go:build network

package luminosity_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/luminosity"
)

// The real curves peak where human vision does.
//
// # Why the peak rather than a checksum
//
// Because it is the one property of these tables that is independently
// known and that no plausible fetch error preserves. Photopic vision peaks
// at 555 nm and scotopic at 507 nm — the Purkinje shift, about 48 nm, which
// is why a red rear light fades and a blue one does not as dusk falls. A
// truncated download, a swapped pair of files or a column read in the wrong
// order all move that peak; a checksum would catch the first and none of the
// rest, and would need updating whenever CVRL touched a byte.
//
// It also checks the two files are not the same file, which is the specific
// failure a copy-paste in the endpoint's Files list would produce.
func TestCurvesPeakWhereVisionDoes(t *testing.T) {
	testutil.RequireReachable(t, "www.cvrl.org:80")

	ids, size := []remote.EndpointID{remote.CVRLLuminosity}, int64(1<<20)
	remote.EnableDownloads(size, ids...)

	peak := func(v luminosity.Vision) float64 {
		t.Helper()

		band, err := luminosity.Open(context.Background(), v)
		if err != nil {
			t.Fatalf("Open %v: %v", v, err)
		}

		if len(band.WavelengthNM) < 100 {
			t.Fatalf("%v curve has %d samples; the published tables have hundreds",
				v, len(band.WavelengthNM))
		}

		best, at := 0.0, 0.0

		for i, r := range band.Response {
			if r > best {
				best, at = r, float64(band.WavelengthNM[i])
			}
		}

		// Both curves are normalised to unity at their peak.
		if best < 0.99 || best > 1.01 {
			t.Errorf("%v curve peaks at %g, want 1", v, best)
		}

		return at
	}

	photopic, scotopic := peak(luminosity.Photopic), peak(luminosity.Scotopic)

	if photopic < 553 || photopic > 557 {
		t.Errorf("photopic peak is at %g nm, want 555", photopic)
	}

	if scotopic < 505 || scotopic > 509 {
		t.Errorf("scotopic peak is at %g nm, want 507", scotopic)
	}

	// The Purkinje shift: rod vision peaks bluer than cone vision.
	if shift := photopic - scotopic; shift < 40 || shift > 56 {
		t.Errorf("the peaks differ by %g nm, want about 48 — that gap is the Purkinje "+
			"shift, and the two files being identical would make it zero", shift)
	}
}
