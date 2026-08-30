//go:build validation

package jpl_test

import (
	"sort"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// mockLinearProvider serves a state that moves linearly from a fixed one.
//
// It exists so the comparison below touches no kernel at all: Horizons' own
// geocentric state goes in, and what comes out is astrogo's apparent-place
// and topocentric arithmetic applied to it. Light-time iteration needs the
// state at slightly earlier instants, which linear motion supplies exactly
// enough for over the milliseconds involved.
type mockLinearProvider struct {
	baseTime time.Time
	pos      vector.Vec3
	vel      vector.Vec3
}

func (m *mockLinearProvider) State(_ eph.ID, t time.Time) (eph.State, error) {
	jd1Req, jd2Req := t.JDParts()
	jd1Base, jd2Base := m.baseTime.JDParts()
	dtDays := (jd1Req - jd1Base) + (jd2Req - jd2Base)

	p := m.pos.Add(m.vel.MulScalar(dtDays))

	return eph.State{Pos: p, Vel: m.vel}, nil
}

func (m *mockLinearProvider) Close() error { return nil }

// TestScientificStability measures the astrometric-to-topocentric pipeline
// against a fixed Horizons corpus, offline.
//
// # What this does and does not validate
//
// Not the ephemeris. The provider above is a linear mock fed with Horizons'
// own geocentric state, so no SPK kernel is read and no integration is
// exercised. What is under test is everything downstream of the state:
// light-time iteration in eph.ApparentState, the IAU 2006/2000A reduction
// chain in coord.Context, observer geodesy, and the topocentric parallax that
// turns a geocentric direction into an alt/az.
//
// The name oversold this before. Astrometric RA and Dec were compared against
// a threshold of 4000 arcseconds — 1.1 degrees — and even then only logged,
// never failed, so that leg was covered by a print statement.
//
// # The contract, and the one leg that deliberately has none
//
// The assertion is on topocentric alt/az, and its bound is the one
// TestObserverPrecisionMatrix derives, for the same reason: Earth orientation
// is the only input the two implementations do not share, and at 15.041
// arcsec of hour angle per second of UT1, 3 arcseconds is a fifth of a second
// of UT1. The two tests reach that figure by different routes — a live matrix
// and a frozen corpus — and agree, at 1.97 and 2.07 arcseconds.
//
// The intermediate place is measured and reported but not contracted, because
// this corpus cannot validate it and saying so is more useful than a number
// that pretends otherwise:
//
//   - astrogo's eph.ApparentState retards the whole geocentric vector, so
//     what it returns is T(t-tau) - E(t-tau). That carries light-time and
//     annual aberration together, which is deliberate: coord's observed path
//     does not apply aberration again, and the 2 arcsecond topocentric
//     agreement is the evidence that nothing double-counts.
//   - Horizons' astrometric column is T(t-tau) - E(t), light-time only. The
//     two therefore differ by Earth's motion over the light time — v/c, the
//     aberration constant — which is measured below at up to 21.2 arcsec
//     against a predicted 20.8 at perihelion.
//   - Horizons' apparent column would carry aberration but is referred to
//     the true equinox of date, while coord.AstrometricToApparent produces
//     CIRS. Those differ by the equation of the origins, which is degrees.
//
// So neither published column is the quantity astrogo computes here.
// Comparing against one anyway and widening a tolerance to cover the gap is
// how a model difference gets buried in a number nobody questions; the gap is
// named and quantified instead. Closing it needs a Horizons column referred
// to ICRF with aberration applied, which the service does not offer.
func TestScientificStability(t *testing.T) {
	c, err := loadCorpus()
	if err != nil {
		t.Fatalf("%v\n  regenerate with: go test -tags=network -run TestGenerateCorpus "+
			"./ephemeris/jpl/validation/ -args -update-corpus", err)
	}

	t.Logf("corpus: %d entries, generated %s at astrogo %s",
		len(c.Entries), c.Manifest.Generated, c.Manifest.Commit)
	t.Logf("  reference:  %s", c.Manifest.Reference)
	t.Logf("  refraction: %s", c.Manifest.Refraction)
	t.Logf("  sampling:   %s", c.Manifest.Sampling)

	for _, note := range c.Manifest.NotPinned {
		t.Logf("  not pinned: %s", note)
	}

	ref := metrology.Reference{
		Kind:    metrology.KindHorizons,
		Name:    "JPL Horizons",
		Version: "OBSERVER + VECTORS, AIRLESS",
		Source:  c.Manifest.Reference,
		Dataset: "corpus/horizons.json, generated " + c.Manifest.Generated,
	}

	// Measured, not contracted — see the doc comment. Collected in a plain
	// slice rather than a Suite because a Suite carries a contract, and a
	// contract invented to fit a known model difference is exactly what this
	// package exists to stop.
	var aberrationGap []float64

	topocentric := metrology.NewSuite("coord.topocentric.corpus", ref,
		metrology.MustContract(3.0, "arcsec",
			"3 arcsec is 0.20 s of UT1 at Earth's rotation rate of 15.041 arcsec/s, and Earth "+
				"orientation is the only input astrogo and Horizons do not share here",
			"IERS Conventions (2010), Earth rotation rate; the same bound TestObserverPrecisionMatrix derives"))

	// Airless on both sides: the corpus manifest records that Horizons sent
	// no APPARENT parameter, so its Az/El are unrefracted.
	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	belowHorizon := 0

	for _, e := range c.Entries {
		cs, err := c.site(e.SiteName)
		if err != nil {
			t.Fatalf("%s: %v", e.key(), err)
		}

		site, err := coord.NewGeodetic(angle.Deg(cs.Lon), angle.Deg(cs.Lat), cs.Height)
		if err != nil {
			t.Fatalf("%s: %v", cs.Name, err)
		}

		obsTime := time.FromJD(e.EpochJDUT, time.UTC)

		mock := &mockLinearProvider{
			baseTime: obsTime,
			pos:      vector.Vec3{X: e.GeoVector[0], Y: e.GeoVector[1], Z: e.GeoVector[2]},
			vel:      vector.Vec3{X: e.GeoVelocity[0], Y: e.GeoVelocity[1], Z: e.GeoVelocity[2]},
		}

		appState, err := eph.ApparentState(mock, eph.ID(e.TargetID), obsTime)
		if err != nil {
			t.Errorf("%s: ApparentState: %v", e.key(), err)

			continue
		}

		appICRS, err := eph.ToICRS(appState.Pos)
		if err != nil {
			t.Errorf("%s: ToICRS: %v", e.key(), err)

			continue
		}

		label := e.TargetName + " @ " + e.SiteName
		scenario := e.key()

		// A great-circle separation rather than separate RA and Dec deltas,
		// so a right-ascension wrap near zero cannot masquerade as a large
		// error and a cos(dec) factor cannot be forgotten.
		aberrationGap = append(aberrationGap, metrology.AngularSeparation(
			appICRS.RA(), appICRS.Dec(),
			angle.Deg(e.Observed.AstroRA), angle.Deg(e.Observed.AstroDec),
		).Arcseconds())

		// A target below the horizon has an alt/az, but comparing one is
		// comparing two extrapolations of a geometry neither implementation
		// is built to report; the precision matrix skips these for the same
		// reason.
		if e.Observed.Elevation <= 0 {
			belowHorizon++

			continue
		}

		observed := coord.NewContext(obsTime, site, atm).GeocentricToObserved(appState.Pos)

		topocentric.Add(metrology.Sample{
			Error: metrology.AngularSeparation(
				observed.Az(), observed.Alt(),
				angle.Deg(e.Observed.Azimuth), angle.Deg(e.Observed.Elevation),
			).Arcseconds(),
			Label: label, Context: scenario,
		})
	}

	t.Logf("%d of %d entries are below the horizon in Horizons' own answer and are compared "+
		"astrometrically only", belowHorizon, len(c.Entries))

	sort.Float64s(aberrationGap)

	t.Logf("apparent-place gap against Horizons' astrometric column (measured, not contracted): "+
		"n=%d p50=%.3f p95=%.3f max=%.3f arcsec — this is Earth's motion over the light time, "+
		"bounded by v/c, which is 20.50 arcsec at mean orbital speed and 20.85 at perihelion",
		len(aberrationGap),
		metrology.Quantile(aberrationGap, 0.50),
		metrology.Quantile(aberrationGap, 0.95),
		metrology.Quantile(aberrationGap, 1.0))

	topocentric.Report(t)
}
