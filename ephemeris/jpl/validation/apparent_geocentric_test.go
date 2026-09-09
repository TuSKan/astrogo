//go:build network

package jpl_test

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// TestApparentAgreesWithHorizonsGeocentric compares astrogo's apparent place
// against Horizons' quantity 2, one stage further down the chain than
// [TestAstrometricAgreesWithHorizonsGeocentric].
//
// # What the extra stage is
//
// The astrometric comparison above validates the ephemeris, the light-time
// solution and the frame, and nothing else — quantity 1 carries no aberration
// and no deflection. Quantity 2 carries both. So this measures exactly what
// [eph.ApparentState] adds: annual aberration, expressed through the retarded
// observer, and gravitational light deflection by the Sun.
//
// Nothing measured the second of those until #263, because until #263
// astrogo did not apply it.
//
// # What it found
//
// Deflection was absent, and the evidence is the shape of the residual rather
// than its size. Against Horizons over 2026, worst case per body:
//
//	body      without deflection   with it
//	Mercury         0.0790"        0.0522"
//	Venus           0.1822"        0.0524"
//	Mars            0.1872"        0.0496"
//	Jupiter         0.6688"        0.0531"
//	Saturn          0.1103"        0.0609"
//
// Afterwards every body sits flat at 0.049 to 0.061 arcseconds with almost no
// scatter — p95 and max within three milliarcseconds of the median — which is
// the constant offset this package already measures in right ascension and
// characterises elsewhere (#260). Before, the tail was the deflection: largest
// where the geometry put a planet nearest the Sun, and Jupiter's worst case
// thirteen times what it is now.
//
// # Why the contract is not tighter
//
// 0.1 arcsecond, which is twice the flat residual and not the residual itself.
// A tolerance pinned to its own measurement can only fail for something larger
// than what has already been seen, which is the one thing it does not need to
// detect — the same argument that reset the topocentric bound in this package.
// What makes 0.1 meaningful is the ratio: it is half of Venus's pre-fix worst
// case and a seventh of Jupiter's, so dropping the deflection again fails this,
// which is the property worth having.
//
// # A note on SOFA
//
// Running the same comparison through SOFA's own Atci13 from the astrometric
// place gives a longer tail than astrogo's: Mercury max 0.2452" against
// 0.0522", Venus 0.1580" against 0.0524". That is not a defect in SOFA. Atci13
// deflects with Ldsun, which treats the source as infinitely distant — correct
// for a star and an overestimate for a body inside the solar system, where the
// light path spends less of its length near the Sun. astrogo uses iauLd with
// the true Sun-to-target geometry, which is what Horizons does too.
func TestApparentAgreesWithHorizonsGeocentric(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	provider, err := jpl.NewProvider(context.Background(), core.Planets, "de441_part-2")
	if err != nil {
		t.Fatalf("de441_part-2 provider: %v", err)
	}

	defer func() { _ = provider.Close() }()

	// The same bodies and the same barycentre-versus-body-centre care as the
	// astrometric comparison: '5' and '6', not '599' and '699' (#253). The Sun
	// is left out — it is the deflecting body, so the term under test does not
	// apply to it.
	bodies := []struct {
		command string
		name    string
		id      core.ID
	}{
		{"199", "Mercury", core.Mercury},
		{"299", "Venus", core.Venus},
		{"499", "Mars", core.Mars},
		{"5", "Jupiter", core.Jupiter},
		{"6", "Saturn", core.Saturn},
	}

	reference := metrology.Reference{
		Kind:           metrology.KindHorizons,
		Name:           "JPL Horizons",
		Version:        "quantity 2 (apparent), DE441",
		Source:         "https://ssd.jpl.nasa.gov/api/horizons.api",
		Dataset:        "CENTER='500@399', APPARENT='AIRLESS', ANG_FORMAT='DEG', EXTRA_PREC='YES'",
		SharedAncestor: "JPL DE441 — both sides read the same ephemeris, by design",
	}

	suite := metrology.NewSuite("ephemeris.apparent.geocentric", reference,
		metrology.MustContract(0.1, "arcsec",
			"twice the flat residual the comparison measures and a seventh of the worst case "+
				"it showed before solar light deflection was applied, so removing that term "+
				"again fails this — a bound set by what it must catch rather than by what has "+
				"already been seen",
			"measured: 5 bodies x 24 epochs across 2026, p50 0.047-0.050 arcsec with p95 "+
				"within 0.003 of it; the offset itself is the right-ascension constant "+
				"characterised in #260"))

	var (
		separations []float64
		dRACosDec   []float64
		dDec        []float64
	)

	for _, body := range bodies {
		rows, err := fetchGeocentricSeries(body.command, body.name, "2",
			"2026-01-01", "2026-12-25", "15d")
		if err != nil {
			testutil.SkipOnUpstreamFailure(t, err)
			t.Fatalf("%s: fetching the geocentric apparent series: %v", body.name, err)
		}

		if len(rows) == 0 {
			t.Fatalf("%s: Horizons returned no rows", body.name)
		}

		var bodySeparations []float64

		for _, row := range rows {
			epoch := time.FromJD(row.jdUT, time.UTC)

			state, err := eph.ApparentState(provider, body.id, epoch)
			if err != nil {
				t.Fatalf("%s: ApparentState: %v", body.name, err)
			}

			gotRA, gotDec := apparentEquinoxOfDate(state.Pos, epoch)

			dRA := wrapDegrees(gotRA-row.raDeg) * math.Cos(gotDec*math.Pi/180)
			dD := gotDec - row.decDeg

			sep := math.Hypot(dRA, dD) * 3600

			suite.Add(metrology.Sample{
				Error:   sep,
				Label:   body.name,
				Context: fmt.Sprintf("JD %.5f UT, geocentric apparent", row.jdUT),
			})

			dRACosDec = append(dRACosDec, dRA*3600)
			dDec = append(dDec, dD*3600)
			separations = append(separations, sep)
			bodySeparations = append(bodySeparations, sep)
		}

		// Per body, and per body is the point: the deflection is a function of
		// how close the geometry brings a planet to the Sun, so an aggregate
		// would average away the very thing that carries the signal.
		sort.Float64s(bodySeparations)

		t.Logf("%-8s n=%2d  p50 %7.4f\"  p95 %7.4f\"  max %7.4f\"",
			body.name, len(bodySeparations),
			bodySeparations[len(bodySeparations)/2],
			bodySeparations[len(bodySeparations)*95/100],
			bodySeparations[len(bodySeparations)-1])
	}

	if len(separations) < 50 {
		t.Fatalf("only %d comparison points; the matrix is too small to say anything",
			len(separations))
	}

	reportSigned := func(label string, xs []float64) float64 {
		var mean float64

		for _, x := range xs {
			mean += x
		}

		mean /= float64(len(xs))

		sorted := append([]float64(nil), xs...)
		sort.Float64s(sorted)

		t.Logf("%-14s signed mean %+8.4f\"  p50 %+8.4f\"  min %+8.4f\"  max %+8.4f\"",
			label, mean, sorted[len(sorted)/2], sorted[0], sorted[len(sorted)-1])

		return mean
	}

	meanRA := reportSigned("dRA*cos(dec)", dRACosDec)
	meanDec := reportSigned("dDec", dDec)

	suite.Report(t)

	// Declination is the clean axis and has to stay clean.
	//
	// The residual this comparison measures lives almost entirely in right
	// ascension — a constant +0.049 arcsec, characterised in #260 — while
	// declination sits at -0.0002. Deflection is not axis-aligned, so it
	// showed in both; pinning the quiet axis is what makes a term reappearing
	// in declination visible instead of averaging into a separation.
	const (
		maxDecBiasArcsec = 0.01
		minRABiasArcsec  = 0.02
		maxRABiasArcsec  = 0.08
	)

	if math.Abs(meanDec) > maxDecBiasArcsec {
		t.Errorf("declination carries a signed bias of %+.4f\", over %.2f\"; the residual here "+
			"is supposed to be a right-ascension constant, so a declination offset is a "+
			"different term", meanDec, maxDecBiasArcsec)
	}

	// And right ascension has to keep carrying it. If this offset ever
	// vanishes, something upstream changed and the explanation on record for
	// it — the equation of the origins and the equinox convention — no longer
	// applies to whatever is being measured.
	if meanRA < minRABiasArcsec || meanRA > maxRABiasArcsec {
		t.Errorf("the right-ascension offset is %+.4f\", outside the %.2f to %.2f\" band this "+
			"comparison has measured since #260; the residual is no longer the constant that "+
			"band describes", meanRA, minRABiasArcsec, maxRABiasArcsec)
	}

	// The spread, not the offset, is what says the deflection is there.
	//
	// The offset is constant and belongs to right ascension; it is measured
	// and explained elsewhere. What the deflection produced when it was
	// missing was *scatter* on top of it, largest for the bodies that pass
	// nearest the Sun. So the assertion is on how far the tail sits above the
	// median: within a hundredth of an arcsecond with the term applied, and
	// six tenths without it for Jupiter.
	sorted := append([]float64(nil), separations...)
	sort.Float64s(sorted)

	p50, worst := sorted[len(sorted)/2], sorted[len(sorted)-1]

	const maxSpreadArcsec = 0.05

	if spread := worst - p50; spread > maxSpreadArcsec {
		t.Errorf("the residual spreads %.4f\" between its median (%.4f\") and its worst "+
			"(%.4f\"), over the %.2f\" this comparison should show.\n"+
			"  A flat residual is a constant offset; a spreading one is a term that depends "+
			"on the geometry, and the one this test was built to catch is solar light "+
			"deflection (#263).",
			spread, p50, worst, maxSpreadArcsec)
	}
}

// apparentEquinoxOfDate turns a geocentric apparent ICRS vector into the right
// ascension and declination Horizons publishes as apparent, in degrees.
//
// Two steps, and the second is the one that is easy to leave out. Rotating by
// the celestial-to-intermediate matrix gives CIRS, whose right ascension is
// measured from the Celestial Intermediate Origin. Horizons publishes the
// classical apparent place, measured from the true equinox, and the two
// origins are separated by the equation of the origins — about 0.33 degrees in
// 2026, so omitting it is a 1200 arcsecond error rather than a subtle one.
//
// eo comes from Atci13 evaluated at the origin because it is a function of the
// date alone; the direction handed in does not affect it.
func apparentEquinoxOfDate(pos vector.Vec3, epoch time.Time) (raDeg, decDeg float64) {
	tt1, tt2 := epoch.TT().JDParts()
	rc2i := gofaext.C2i06a(tt1, tt2)

	u := pos.Unit()

	cirs := vector.V3(
		rc2i[0][0]*u.X+rc2i[0][1]*u.Y+rc2i[0][2]*u.Z,
		rc2i[1][0]*u.X+rc2i[1][1]*u.Y+rc2i[1][2]*u.Z,
		rc2i[2][0]*u.X+rc2i[2][1]*u.Y+rc2i[2][2]*u.Z,
	)

	var spherical coord.Astrometric

	spherical.FromUnitVector(cirs)

	_, _, eo := gofaext.Atci13(0, 0, 0, 0, 0, 0, tt1, tt2)

	return angle.Rad(spherical.RA().Radians() - eo).Wrap360().Degrees(),
		spherical.Dec().Degrees()
}
