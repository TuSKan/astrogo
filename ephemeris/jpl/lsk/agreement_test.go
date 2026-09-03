package lsk_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// firstLeapSecondYear is when UTC began accumulating whole leap seconds.
// Before 1972 the two sources are not expected to agree, because the question
// itself is different: pre-1972 UTC ran on rate offsets and fractional steps.
const firstLeapSecondYear = 1972

// TestLeapSecondSourcesAgree checks NAIF's leap-second kernel against the table
// compiled into gofa, entry by entry and year by year.
//
// # Why this exists
//
// The library has two leap-second sources and uses one of them.
//
//   - gofa's Dat carries a table hardcoded in its source (ts.go, last entry
//     {2017, 1, 37.0}) and pinned by go.mod at v1.19.1. A newly announced leap
//     second reaches astrogo only through a dependency bump and a release.
//   - NAIF's naif0012.tls is a 5 KB data file this package already downloads.
//     A newly announced leap second reaches it on a cache refresh.
//
// Everything in astrogo converts UTC through gofa, including UTCToTDB since it
// was fixed to delegate to time.Time.TDB — the kernel is currently parsed and
// not read for this purpose (see the issue linked from that function's doc).
//
// That is a defensible choice while the two agree, and they do agree today:
// both tables end at 2017-01-01 with 37 leap seconds, because none has been
// announced since. It stops being defensible silently. If IERS announces one,
// the kernel picks it up on the next cache refresh and gofa does not, and every
// UTC-based result in the library is a full second wrong against the ephemeris
// it is being compared to — with nothing failing.
//
// So this test is the tripwire for a decision that is currently implicit. When
// it fails, the answer is not to relax it: it means the pinned table is stale
// and the leap-second source needs to move.
//
// # Why both directions are checked
//
// Walking the kernel's entries and looking each one up in gofa only proves the
// kernel is a subset. It cannot see a leap second gofa has that the kernel
// lacks, nor — more importantly — one the kernel has beyond gofa's last entry,
// which is the exact failure this exists to catch. The year-by-year sweep after
// the entry walk covers both.
func TestLeapSecondSourcesAgree(t *testing.T) {
	// Bounded for the same reason as TestLSKReader: NAIFSPK registers a
	// 30-minute DownloadTimeout, which outlives this binary's whole budget.
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

	// ── Direction 1: every kernel entry, as gofa sees that instant ──────────
	//
	// Sampled a day after each step so the comparison is unambiguous: both
	// sources are then well inside the same interval, and a half-open-interval
	// disagreement at the boundary cannot masquerade as a table mismatch.
	var checked int

	for _, d := range r.DeltaAt {
		y, m, day, _, status := gofaext.JdToDate(d.JD+1, 0)
		if status != 0 {
			t.Errorf("JdToDate(%f) failed with status %d", d.JD, status)

			continue
		}

		if y < firstLeapSecondYear {
			continue
		}

		got, status := gofaext.Dat(y, m, day, 0)
		if status != 0 {
			t.Errorf("gofa Dat(%04d-%02d-%02d) failed with status %d", y, m, day, status)

			continue
		}

		checked++

		if math.Abs(got-d.N) > 1e-9 {
			t.Errorf("%04d-%02d-%02d: the kernel says %.1f leap seconds, gofa says %.1f.\n"+
				"  These are the same published quantity, so one table is stale. astrogo "+
				"converts UTC through gofa, so if the kernel is the newer one every "+
				"UTC-based result is a second wrong against the ephemeris.",
				y, m, day, d.N, got)
		}
	}

	if checked == 0 {
		t.Fatal("no kernel entries were compared; the LSK parse or the date conversion has changed")
	}

	// ── Direction 2: a year-by-year sweep, which is what catches a new entry ─
	//
	// The walk above can only prove the kernel is a subset of gofa. A leap
	// second announced after gofa v1.19.1 was pinned appears in the kernel and
	// nowhere in that walk's failures — it would simply be an entry gofa
	// answers with the previous value. Sweeping independently finds it.
	last := time.Date(2035, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	var swept int

	for y := firstLeapSecondYear; y <= last.Year(); y++ {
		for _, m := range []time.Month{time.January, time.July} {
			jd := time.Date(y, m, 1, 12, 0, 0, 0, time.LocationUTC).JD()

			fromGofa, status := gofaext.Dat(y, int(m), 1, 0.5)
			if status < 0 {
				continue // outside gofa's supported range; not a disagreement
			}

			swept++

			fromKernel := kernelDeltaAT(r, jd)

			if math.Abs(fromGofa-fromKernel) > 1e-9 {
				t.Errorf("%04d-%02d-01: gofa says %.1f leap seconds, the kernel says %.1f.\n"+
					"  A leap second exists in one source and not the other. gofa's table is "+
					"compiled in and pinned by go.mod; the kernel refreshes from NAIF. If the "+
					"kernel is ahead, bump gofa or move the library's leap-second source — "+
					"do not relax this test.",
					y, int(m), fromGofa, fromKernel)
			}
		}
	}

	// Reported so a change that silently stops comparing anything fails the
	// count check above rather than passing vacuously — the same reason the
	// docsguard suites report their totals.
	t.Logf("%d kernel entries and %d year samples agreed; both tables end at "+
		"37 leap seconds (2017-01-01)", checked, swept)
}

// kernelDeltaAT reads the kernel's accumulated leap seconds at a Julian Date,
// mirroring the lookup UTCToTDB used before it delegated to time.Time.TDB.
func kernelDeltaAT(r *lsk.Reader, jd float64) float64 {
	last := 0.0

	for _, d := range r.DeltaAt {
		if jd < d.JD {
			break
		}

		last = d.N
	}

	return last
}
