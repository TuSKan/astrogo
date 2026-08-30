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
// TestPreLeapSecondEpochsMeasureTheClockNotTheEphemeris. Restricted to the era
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
// # Why it starts at 1972 and not at the table's own 1800
//
// Because before 1972 this comparison stops measuring the ephemeris and starts
// measuring the clock. The two sides reach a TDB instant by different routes —
// the kernel path through a NAIF leap-second kernel, the analytical path
// through this module's own delta-T model — and outside the leap-second era
// those routes disagree. The disagreement is a time offset, and a time offset
// on the Earth's orbit is a position error of 29.8 km for every second.
//
// It is not subtle once looked for. The Sun's maximum residual by year:
//
//	1971   302.3 km
//	1972     7.9 km
//
// One year apart, a factor of 38, at exactly the boundary where UTC became
// leap-second based. Before it the residual grows smoothly backwards — 1,056 km
// at 1900 — and after it the whole span to 2100 sits between 3 and 9 km, which
// is Epv00's published accuracy of 11.2 km and nothing else.
//
// Every planet inherits the same term, because a geocentric planet position is
// the heliocentric one minus the Earth's. It is negligible against Jupiter's
// 300,000 km bound and decisive against Mercury's 2,445 km one — which is how
// it came to be mistaken for the reference ephemeris and nearly cost this suite
// a doubled bound. See [planetSeries.contract] and
// TestPreLeapSecondEpochsMeasureTheClockNotTheEphemeris.
//
// The window still ends at 2100, the table's own limit: sampling past it would
// measure against a bound that was never claimed there.
func planetEpochs() []time.Time {
	const (
		firstYear = firstLeapSecondYear
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

// TestPreLeapSecondEpochsMeasureTheClockNotTheEphemeris pins the reason
// [planetEpochs] starts where it does.
//
// A window is the easiest thing in a validation suite to widen without
// thinking — it looks like more evidence, and more evidence is usually better.
// Here it is not: reaching back one year past 1972 replaces an 8 km ephemeris
// residual with a 302 km timekeeping one, and every statistic in the suite
// then describes the disagreement between a leap-second kernel and a delta-T
// model rather than anything about SOFA or DE440.
//
// This test measures the step across that boundary. If it ever vanishes, the
// two time paths have been reconciled and the window can be widened
// deliberately — which is a decision worth making on purpose rather than
// discovering as a mysteriously loose contract.
func TestPreLeapSecondEpochsMeasureTheClockNotTheEphemeris(t *testing.T) {
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

	// Inside the era, the residual must be Epv00's own accuracy. Its
	// documented maximum against DE405 is 11.2 km; 25 km leaves room for
	// DE440 without admitting a timekeeping term.
	if after > 25 {
		t.Errorf("residual in %d is %.1f km, expected Epv00's own ~8 km — "+
			"the leap-second era is no longer clean", firstLeapSecondYear, after)
	}

	// Outside it, the step must still be there. If this fails the suite could
	// honestly sample a wider window, which is a change to make deliberately.
	if before < 10*after {
		t.Errorf("residual in %d is %.1f km, only %.1fx the %.1f km inside the era; "+
			"the pre-1972 timekeeping term appears to be gone, so planetEpochs could "+
			"reach further back — verify and widen it on purpose",
			firstLeapSecondYear-1, before, before/after, after)
	}
}
