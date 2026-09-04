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

// TestUTCToETAgreesWithTheJulianDatePath checks the new conversion against the
// one it replaces, to the precision the old one had.
//
// The two must not differ by more than the old path could represent: anything
// larger would mean the arithmetic changed rather than just its conditioning.
// This is what distinguishes "kept more bits" from "computes something else".
func TestUTCToETAgreesWithTheJulianDatePath(t *testing.T) {
	r := openKernel(t)

	// The old path's own quantum, measured at 32.8 µs; a little headroom over
	// it, since that figure varies slightly with epoch.
	const toleranceSeconds = 100e-6

	for _, year := range []int{1972, 1990, 2010, 2026, 2040} {
		when := time.Date(year, time.June, 15, 12, 0, 0, 0, time.LocationUTC)

		// The deprecated path is the subject of this comparison.
		viaJD := lsk.TDBToET(lsk.UTCToTDB(when, r))
		direct := lsk.UTCToET(when, r)

		if d := math.Abs(direct - viaJD); d > toleranceSeconds {
			t.Errorf("year %d: UTCToET gives %.9f s and the Julian-date path %.9f s, "+
				"differing by %.1f microseconds.\n  Removing the epoch earlier should "+
				"keep more bits of the same value, not compute a different one.",
				year, direct, viaJD, d*1e6)
		}
	}
}

// TestUTCToETDelegatesOutsideTheKernelsEra is the two-part counterpart of the
// check on UTCToTDB: an epoch the DELTA_AT table cannot speak for must still
// reach time's historical Delta-T rather than being answered with no offset.
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
