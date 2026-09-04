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

// openKernel is the shared setup for the tests in this file.
func openKernel(t *testing.T) *lsk.Reader {
	t.Helper()

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

	return r
}

// TestUTCToETResolvesSubMicrosecondSteps pins the precision the two-part
// conversion exists to keep.
//
// # What was lost
//
// The old path summed the two-part Julian Date and only then subtracted the
// epoch. At a modern epoch a Julian Date is about 2.46e6, where one ULP of a
// float64 is 4.657e-10 days — **40 microseconds**. So the ET handed to the SPK
// evaluator was quantised: measured, the smallest offset it could resolve was
// 32.8 µs, and a caller advancing the epoch by 1 µs or 10 µs got back exactly
// the same number.
//
// That is 3.4 cm of lunar motion and 25 cm of ISS track, and the Moon figure
// sits at the level of the 33 mm this library claims against Horizons.
//
// It also made a comparison impossible. Measuring the kernel's Moyer model
// against time's Fairhead & Bretagnon series produced readings of exactly 0.0
// or exactly one ULP at every epoch, because the two models differ by about
// 30 µs and the API could not represent the difference. Any future work
// choosing between them has to keep this precision or it is measuring the
// float.
//
// # The bound
//
// One microsecond, comfortably inside the 0.128 µs measured and far below the
// 32.8 µs that was lost. A microsecond is 1 mm of lunar motion, so this asserts
// the conversion is no longer the limiting factor in anything this library
// claims.
func TestUTCToETResolvesSubMicrosecondSteps(t *testing.T) {
	r := openKernel(t)

	base := time.Date(2026, time.June, 15, 12, 0, 0, 0, time.LocationUTC)
	baseET := lsk.UTCToET(base, r)

	const wantResolvable = time.Duration(1000) // 1 µs in nanoseconds

	shifted := lsk.UTCToET(base.Add(wantResolvable), r)
	if shifted == baseET {
		t.Errorf("advancing the epoch by 1 microsecond did not change ET at all.\n" +
			"  The conversion is quantised — check that the epoch is subtracted from " +
			"the Julian day number before the fraction is added back, rather than after.")
	}

	// And the step it produces is the step it was given.
	if got := (shifted - baseET) * 1e6; math.Abs(got-1.0) > 0.5 {
		t.Errorf("a 1 microsecond step produced %.3f microseconds of ET", got)
	}
}

// TestUTCToETDelegatesOutsideTheKernelsEra pins that an epoch the DELTA_AT
// table cannot speak for still reaches time's historical Delta-T rather than
// being answered with no offset at all.
//
// The table begins at 1972-01-01, because that is when UTC began accumulating
// whole leap seconds. Before it the offset is Delta-T — the Espenak & Meeus
// (2006) polynomials time.DeltaT implements — which is about 10,500 s at year
// 1. Driving the conversion from the kernel without checking its era applied
// no offset at all, and surfaced as ~180 minute errors across the AstroPixels
// year-0001 lunar phases.
func TestUTCToETDelegatesOutsideTheKernelsEra(t *testing.T) {
	r := openKernel(t)

	const toleranceSeconds = 1e-3

	for _, year := range []int{1, 1000, 1900, 1971} {
		when := time.Date(year, time.January, 5, 12, 0, 0, 0, time.LocationUTC)

		d1, d2 := when.TDB().JDParts()
		fromTime := ((d1 - 2451545.0) + d2) * 86400.0

		if d := math.Abs(lsk.UTCToET(when, r) - fromTime); d > toleranceSeconds {
			t.Errorf("year %d: UTCToET and time.TDB differ by %.1f s (%.1f min).\n"+
				"  This epoch predates the DELTA_AT table, so the kernel must not be "+
				"consulted.", year, d, d/60)
		}
	}
}

// tdbMinusUTC is TDB − UTC in seconds at t, in the two-part form.
//
// The tests below used to compute this as (UTCToTDB(t) - t.JD()) * 86400 — a
// difference of two numbers near 2.46e6, so catastrophic cancellation on top
// of the 40 microsecond quantisation that motivated UTCToET in the first
// place. Subtracting in seconds past J2000 avoids both.
func tdbMinusUTC(t time.Time, r *lsk.Reader) float64 {
	d1, d2 := t.UTC().JDParts()
	secUTC := ((d1 - 2451545.0) + d2) * 86400.0

	return lsk.UTCToET(t, r) - secUTC
}
