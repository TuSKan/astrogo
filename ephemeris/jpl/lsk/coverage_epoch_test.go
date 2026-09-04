package lsk_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// TestUTCToTDBDelegatesOutsideTheKernelsEra pins that an epoch the DELTA_AT
// table cannot speak for is handed to time rather than answered with a
// zero-leap-second offset.
//
// # The regression
//
// The DELTA_AT table begins at 1972-01-01, because that is when UTC began
// accumulating whole leap seconds. Before it, leap seconds do not apply and the
// offset between atomic and rotational time is the historical Delta-T instead —
// the Espenak & Meeus (2006) polynomials that time.DeltaT implements.
// Driving the conversion from the kernel without checking its era therefore
// applied *no* offset at all to ancient epochs — measured against time.TDB:
//
//	year    1: -174.9 min
//	year 1000:  -25.5 min
//	year 1900:   +0.6 min
//	year 1972 onward: exact
//
// It surfaced as ~180 minute errors across every lunar phase in the AstroPixels
// year-0001 comparison, in the integration tier — three tiers of green tests
// away from the change that caused it, because nothing in the default suite
// converts an ancient epoch through a kernel.
//
// # What is asserted
//
// Agreement with time.TDB outside the kernel's era, to a millisecond. Not a
// physical tolerance: outside its coverage the kernel is not consulted at all,
// so the two are the same computation and the only permitted difference is the
// float cost of the Julian Date round-trip.
func TestUTCToTDBDelegatesOutsideTheKernelsEra(t *testing.T) {
	fetchCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	prov, err := jpl.NewProvider(fetchCtx, core.Planets, "de440s")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("setup failed: %v", err)
	}

	t.Cleanup(func() {
		if err := prov.Close(); err != nil {
			t.Errorf("close provider: %v", err)
		}
	})

	ctx := context.Background()

	bucket, prefix, err := remote.CacheDir(ctx, remote.NAIFLSK)
	testutil.AssertNoError(t, err)

	f, err := bucket.NewReader(ctx, prefix+"lsk/naif0012.tls", nil)
	if err != nil {
		t.Fatalf("open cached LSK: %v", err)
	}

	r, err := lsk.NewReader(f)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("close reader: %v", err)
		}
	})

	// A millisecond, which is far above the ~40 microsecond floor a summed
	// Julian Date imposes and far below the minutes the regression cost.
	const toleranceSeconds = 1e-3

	for _, year := range []int{1, 1000, 1600, 1900, 1958, 1971} {
		utc := time.Date(year, time.January, 5, 12, 0, 0, 0, time.LocationUTC)

		fromKernel := lsk.UTCToTDB(utc, r)

		d1, d2 := utc.TDB().JDParts()
		fromTime := d1 + d2

		if diff := math.Abs(fromKernel-fromTime) * 86400; diff > toleranceSeconds {
			t.Errorf("year %d: UTCToTDB and time.TDB differ by %.1f s (%.1f min).\n"+
				"  This epoch predates the DELTA_AT table, so the kernel must not be "+
				"consulted — applying a zero-leap-second offset here drops the "+
				"historical Delta-T entirely.", year, diff, diff/60)
		}
	}
}
