//go:build validation

package jpl_test

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/time"
)

// planetSeries is one row of SOFA's own published error table for Plan94,
// together with the distance that turns its angular columns into a length.
//
// Plan94 is a truncated analytical series, not a Kepler propagation and not
// an integration, so its error is bounded rather than growing — which is what
// makes a fixed contract the right shape for it at all.
type planetSeries struct {
	id core.ID

	// lonArcsec, latArcsec and radiusKM are the maximum absolute
	// differences Plan94's documentation reports against DE200 over
	// 1800-2100, in heliocentric longitude, latitude and radius vector.
	lonArcsec, latArcsec, radiusKM float64

	// aphelionAU is the largest heliocentric distance the body reaches.
	// The angular columns above are angles; converting them into a length
	// needs a distance, and the largest one is the only choice that cannot
	// understate the bound.
	aphelionAU float64
}

// planetSeriesTable is SOFA's table, transcribed:
//
//	Comparisons against DE200 over the interval 1800-2100 gave the
//	following maximum absolute differences.
//
//	              L (arcsec)   B (arcsec)     R (km)
//	 Mercury            7            1            500
//	 Venus              7            1           1100
//	 EMB                9            1           1300
//	 Mars              26            1           9000
//	 Jupiter           78            6          82000
//	 Saturn            87           14         263000
//	 Uranus            86            7         661000
//	 Neptune           11            2         248000
//
// The EMB row is unused: astrogo never asks Plan94 for the Earth, and a
// geocentric Earth is degenerate. Aphelion distances are a(1+e) from the same
// Simon et al. mean elements the series is built on.
func planetSeriesTable() []planetSeries {
	return []planetSeries{
		{core.Mercury, 7, 1, 500, 0.4667},
		{core.Venus, 7, 1, 1100, 0.7282},
		{core.Mars, 26, 1, 9000, 1.6660},
		{core.Jupiter, 78, 6, 82_000, 5.4547},
		{core.Saturn, 87, 14, 263_000, 10.0536},
		{core.Uranus, 86, 7, 661_000, 20.1972},
		{core.Neptune, 11, 2, 248_000, 30.3276},
	}
}

// bound converts one row into the quantity this suite actually measures: the
// length of a position-vector difference, in AU.
//
// The published columns are three orthogonal components of one error — two
// angular, one radial — so the vector length is their root-sum-square, with
// the angles carried out to the body's aphelion. Combining independently
// attained maxima this way overstates the simultaneous error, which is the
// safe direction for a bound and is stated here rather than left implicit.
func (p planetSeries) bound() float64 {
	const arcsecPerRadian = 206264.806247096355

	distKM := p.aphelionAU * kmPerAU

	lon := p.lonArcsec / arcsecPerRadian * distKM
	lat := p.latArcsec / arcsecPerRadian * distKM

	return math.Sqrt(lon*lon+lat*lat+p.radiusKM*p.radiusKM) / kmPerAU
}

// contract states that bound together with the reasoning that produced it.
//
// # Why it is not the measured value
//
// Because a bound taken from a measurement cannot fail for the reason it
// exists. That fault was present in two places in this repository before the
// generated table, and avoiding it is the whole argument of internal/metrology.
// Every number below comes from SOFA's documentation of its own routine; none
// comes from a run of this suite.
//
// # Why the published figure is not used unmodified
//
// The table states heliocentric longitude, latitude and radius; this suite
// measures the length of a geocentric position difference. Those are different
// quantities, and not by a little: for Jupiter the radius column is 82,000 km
// while the longitude column carried out to aphelion is 309,000 km. Contracting
// on R alone would demand four times the agreement SOFA claims for its own
// routine, and the suite would fail on a correct evaluation.
//
// Geocentric rather than heliocentric adds Epv00's error to every sample,
// which its documentation bounds at 11.2 km against DE405. Against the
// smallest bound here — Mercury's — that is under half a percent, and it is
// not subtracted out.
//
// # Why there is no factor for the reference ephemeris, after all
//
// The Sun and Moon suite doubles its published figures because those are
// quoted against DE405 and ELP/MPP02 while the comparison runs against DE440.
// The same argument would apply here — this table is quoted against DE200 —
// and an intermediate version of this file applied it, because the undoubled
// bound failed: Mercury measured 4,012 km against a 2,445 km bound.
//
// That was the wrong conclusion from a real measurement. The excess was not
// DE200 disagreeing with DE440; it was the sampling window reaching back
// before 1972, where the two sides of the comparison do not agree on what time
// it is. See [planetEpochs] and
// TestNoTimekeepingStepAtTheLeapSecondBoundary. Restricted to the era
// where the time scale is unambiguous, every body passes the undoubled bound
// with 1.3x to 1.9x in hand, and the contract stays a tight one.
//
// The lesson is the one the package doc keeps making: the first fix that turns
// a suite green is not necessarily the right one, and doubling a bound is the
// move most likely to hide the reason it failed.
func (p planetSeries) contract() metrology.Contract {
	return metrology.MustContract(p.bound(), "AU",
		fmt.Sprintf("root-sum-square of Plan94's published maximum absolute differences against "+
			"DE200 over 1800-2100 — L %g arcsec and B %g arcsec carried out to %g AU aphelion, "+
			"with R %g km — because this suite measures a position-vector length while the table "+
			"states two angles and a radius. Not doubled for DE200 against DE440: measured over "+
			"the leap-second era every body sits inside the undoubled bound",
			p.lonArcsec, p.latArcsec, p.aphelionAU, p.radiusKM),
		"gofa ephem.go, Plan94 note 5, maximum absolute differences against DE200")
}

// isoDate is the layout for a sample label. Spelled out rather than taken
// from the standard library, which this module does not import outside its
// own time package.
const isoDate = "2006-01-02"

// planetEpochs samples the interval SOFA's table is quoted over.
//
// Quarterly rather than at three named instants, because a maximum over three
// samples is not a maximum, and the distribution is the point: the series
// error is periodic in each body's own terms, so a handful of epochs can land
// anywhere in it. The Sun and Moon suite had exactly that problem — one of its
// three epochs used to be the wall clock, so its margin was re-rolled on every
// run and any failure was unreproducible.
//
// # Why it once started at 1972, and no longer does
//
// It used to begin at the leap-second boundary, because before 1972 this
// comparison stopped measuring the ephemeris and started measuring the clock.
// The two sides reached a TDB instant by different routes — the kernel path
// through the leap-second kernel, the analytical path through this module's own
// delta-T model — and outside the leap-second era those routes disagreed. A
// time offset on the Earth's orbit is a position error of 29.8 km per second,
// so the Sun's residual read 302.3 km at 1971 against 7.9 km at 1972, a factor
// of 38 at exactly that boundary, growing to 1,056 km by 1900.
//
// The cause was in the conversion, not the ephemeris: the kernel path applied
// no offset at all before its DELTA_AT table began, dropping the historical
// delta-T entirely. It now delegates to time for those epochs, and the term is
// gone. Measured across the whole widened window, the Sun's residual is 4 to 9
// km with no step at the boundary — 6.4 km at 1900 where it was 1,056, and 7.3
// km at 1971 where it was 302.3.
//
// So the window is the one the table is actually quoted over. Every body holds
// its contract across it, with the tightest margin 1.13x for Jupiter and the
// pre-1972 worst cases comparable to the post-1972 ones rather than
// systematically larger:
//
//	Mercury  1972+ 8.9e-06   1800-1971 7.9e-06   contract 1.6e-05
//	Jupiter  1972+ 1.2e-03   1800-1971 1.9e-03   contract 2.1e-03
//	Neptune  1972+ 1.7e-03   1800-1971 1.9e-03   contract 2.3e-03
//
// Every planet inherited that timekeeping term, because a geocentric planet
// position is the heliocentric one minus the Earth's. It was negligible against
// Jupiter's bound and decisive against Mercury's — which is how it came to be
// mistaken for the reference ephemeris and nearly cost this suite a doubled
// bound. See [planetSeries.contract] and
// TestNoTimekeepingStepAtTheLeapSecondBoundary, which now guards the boundary
// from the other side.
//
// The window still ends at 2100, the table's own limit: sampling past it would
// measure against a bound that was never claimed there. 1800 is that same
// limit at the other end.
func planetEpochs() []time.Time {
	const (
		firstYear = 1800
		lastYear  = 2100
	)

	quarters := []time.Month{time.January, time.April, time.July, time.October}

	out := make([]time.Time, 0, (lastYear-firstYear+1)*len(quarters))

	for y := firstYear; y <= lastYear; y++ {
		for _, mo := range quarters {
			out = append(out, time.Date(y, mo, 1, 0, 0, 0, 0, time.LocationUTC))
		}
	}

	return out
}

// TestSOFAPlanetsAgainstDE440 measures what astrogo's offline planetary
// positions are actually worth.
//
// # Why this suite did not exist
//
// The generated table carried ephemeris.sofa.sun and ephemeris.sofa.moon and
// nothing else, so the seven bodies a caller is most likely to ask for had no
// measured row at all. eph.Default() is the provider a caller gets without a
// kernel, and what its planets were good for was neither claimed nor bounded
// anywhere in the repository.
//
// # What it does and does not establish
//
// The two models are genuinely independent — a JPL kernel read by this
// repository's Chebyshev decoder on one side, gofa's truncated series on the
// other — so a disagreement here is real rather than a rounding artefact. But
// gofa is astrogo's own dependency rather than a third party's implementation,
// so a fault in how astrogo drives gofa is invisible; only a fault in how it
// drives the kernel would show. The shared ancestry is recorded on the
// reference so the generated table says so instead of leaving a reader to
// work it out.
func TestSOFAPlanetsAgainstDE440(t *testing.T) {
	for _, body := range planetSeriesTable() {
		t.Run(body.id.String(), func(t *testing.T) {
			suite := metrology.NewSuite("ephemeris.sofa."+strings.ToLower(body.id.String()),
				sofaReference(), body.contract())

			p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
			if err != nil {
				metrology.NotVerified(t, "the JPL provider could not be built: "+err.Error(), suite)
			}

			defer func() { _ = p.Close() }()

			sofa := eph.Default()

			for _, at := range planetEpochs() {
				want, werr := p.State(body.id, at)
				if werr != nil {
					t.Fatalf("DE440 State(%s) at %s: %v", body.id, at.Format(isoDate), werr)
				}

				got, gerr := sofa.State(body.id, at)
				if gerr != nil {
					t.Fatalf("SOFA State(%s) at %s: %v", body.id, at.Format(isoDate), gerr)
				}

				diff := got.Pos.Sub(want.Pos).Norm()

				suite.Add(metrology.Sample{
					Error: diff,
					Label: body.id.String(),
					Context: fmt.Sprintf("%s, |DE440 - SOFA| = %.0f km",
						at.Format(isoDate), diff*kmPerAU),
				})
			}

			suite.Report(t)
		})
	}
}

// firstLeapSecondYear is when UTC became leap-second based, and therefore the
// first year in which both sides of these comparisons agree on what time it is.
const firstLeapSecondYear = 1972

// TestNoTimekeepingStepAtTheLeapSecondBoundary guards the boundary
// [planetEpochs] used to stop at.
//
// # What this used to assert, and why it changed
//
// It used to assert the opposite: that a step existed. Reaching one year past
// 1972 replaced an 8 km ephemeris residual with a 302 km timekeeping one,
// because the two sides reached a TDB instant by different routes and the
// kernel path applied no offset at all before its DELTA_AT table began. The
// window was restricted to dodge that, and this test held the restriction
// honest — with the note that if the step ever vanished, the window could be
// widened deliberately.
//
// It vanished. The conversion now delegates to time for epochs the kernel
// cannot speak for, so the historical delta-T is applied instead of nothing,
// and the window has been widened to the 1800 the table is quoted over.
//
// So the assertion inverts. A step reappearing means a timekeeping term has
// come back, and every statistic in the suite would then describe the
// disagreement between two time paths rather than anything about SOFA or
// DE440.
//
// # The threshold
//
// Three times, which distinguishes the two things that can produce a step.
//
// Epv00 degrades gently outside its quoted 1900-2100: measured, the Sun's
// worst residual is 7.8 km inside the leap-second era and 11.7 km across
// 1800-1971, a ratio of 1.5. A timekeeping term is not gentle — the one this
// replaces was a factor of 38, and one second of Earth orbital motion is
// 29.8 km against an 8 km signal. Nothing lands between.
func TestNoTimekeepingStepAtTheLeapSecondBoundary(t *testing.T) {
	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
	if err != nil {
		t.Skipf("the JPL provider could not be built: %v", err)
	}

	defer func() { _ = p.Close() }()

	sofa := eph.Default()

	worstIn := func(year int) float64 {
		t.Helper()

		var worst float64

		for mo := 1; mo <= 12; mo++ {
			at := time.Date(year, time.Month(mo), 1, 0, 0, 0, 0, time.LocationUTC)

			want, werr := p.State(core.Sun, at)
			if werr != nil {
				t.Fatalf("DE440 State(Sun) in %d: %v", year, werr)
			}

			got, gerr := sofa.State(core.Sun, at)
			if gerr != nil {
				t.Fatalf("SOFA State(Sun) in %d: %v", year, gerr)
			}

			worst = math.Max(worst, got.Pos.Sub(want.Pos).Norm()*kmPerAU)
		}

		return worst
	}

	before := worstIn(firstLeapSecondYear - 1)
	after := worstIn(firstLeapSecondYear)

	t.Logf("Sun residual: %d %.1f km, %d %.1f km — a factor of %.0f across the boundary",
		firstLeapSecondYear-1, before, firstLeapSecondYear, after, before/after)

	// Both sides must be Epv00's own accuracy. Its documented maximum against
	// DE405 is 11.2 km; 25 km leaves room for DE440 on either side of the
	// boundary without admitting a timekeeping term.
	for _, side := range []struct {
		year     int
		residual float64
	}{
		{firstLeapSecondYear - 1, before},
		{firstLeapSecondYear, after},
	} {
		if side.residual > 25 {
			t.Errorf("residual in %d is %.1f km, expected Epv00's own ~8 km",
				side.year, side.residual)
		}
	}

	// And there must be no step across it. Anything approaching a factor of
	// three is a time offset rather than a series error: one second of Earth
	// orbital motion is 29.8 km against an 8 km signal, and the term this
	// replaces was a factor of 38.
	if before > 3*after {
		t.Errorf("residual jumps from %.1f km in %d to %.1f km in %d, a factor of %.1f.\n"+
			"  That is a timekeeping term returning, not a series error — check that "+
			"epochs before the DELTA_AT table still delegate to time's historical "+
			"delta-T rather than being given no offset at all.",
			after, firstLeapSecondYear, before, firstLeapSecondYear-1, before/after)
	}
}
