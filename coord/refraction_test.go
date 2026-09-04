package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// refractionScene builds the two Contexts these tests compare: one with an
// atmosphere and one without, identical in every other respect.
//
// Differencing them is what isolates refraction. Comparing absolute altitudes
// between the vector and stellar pipelines would not: they take different
// inputs — a geometric position vector against a catalog star direction — and
// the stellar path applies annual aberration, which is up to 20 arcsec and has
// nothing to do with the atmosphere. Measured, that leaves about 9.5 arcsec
// between them even when refraction agrees perfectly.
func refractionScene(t *testing.T) (withAtm, noAtm *coord.Context) {
	t.Helper()

	site, err := coord.NewGeodetic(angle.Deg(-46.6), angle.Deg(-23.5), 800)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	epoch := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)

	atm := atmosphere.AtAltitude(800)

	dry := atm
	dry.Pressure = 0

	return coord.NewContext(epoch, site, atm), coord.NewContext(epoch, site, dry)
}

// farVector is a geocentric position vector pointing at an ICRS direction, far
// enough away that the observer offset does not measurably change it.
func farVector(c coord.ICRS) vector.Vec3 {
	u := c.ToUnitVector()
	return vector.Vec3{X: u.X * 1e9, Y: u.Y * 1e9, Z: u.Z * 1e9}
}

// TestRefractionMatchesTheStellarPipeline is the guard the +7028° defect
// needed and did not have.
//
// # What went wrong
//
// GeocentricToObserved wrote out what looked like SOFA's refraction model —
// Refa·tan(z) + Refb·tan³(z) — guarded by z < 91° and dR > 0. It was wrong
// three ways at once, and no test compared it against anything.
//
// tan(z) diverges at the horizon, not at the 91° the guard allowed, so just
// below it the cubic term flipped sign to about +61 rad and the dR > 0 test
// passed it through: an altitude of **+7028°** at a geometric −0.076°. Between
// roughly 0° and 2° the cubic cancelled the linear term instead, dR went
// non-positive, and refraction was dropped entirely — 0.000° where the stellar
// path applied 0.16°. And the model was the uncorrected series, missing the
// Newton-Raphson step Atioq applies even where it converges.
//
// # Why this compares increments
//
// The two pipelines answer different questions and their absolute altitudes
// legitimately differ (see refractionScene). Differencing each against its own
// atmosphere-free Context isolates the one quantity that must match.
func TestRefractionMatchesTheStellarPipeline(t *testing.T) {
	t.Parallel()

	withAtm, noAtm := refractionScene(t)

	// One arcsecond, which is the bound #100 asked for. Measured agreement is
	// far tighter — a few milliarcsec below the clamp, under 0.2 arcsec above
	// it — so this has room for platform rounding without admitting a
	// regression.
	const toleranceArcsec = 1.0

	var checked int

	for raDeg := 0.0; raDeg < 360.0; raDeg += 0.25 {
		icrs := coord.NewICRS(angle.Deg(raDeg), angle.Deg(-20))
		far := farVector(icrs)

		geometric := noAtm.GeocentricToObserved(far).Alt().Degrees()
		if geometric < -5 || geometric > 5 {
			continue
		}

		checked++

		vectorRefraction := withAtm.GeocentricToObserved(far).Alt().Degrees() - geometric

		sr, err := withAtm.ICRSToAltAz(icrs)
		if err != nil {
			t.Fatalf("ICRSToAltAz: %v", err)
		}

		sg, err := noAtm.ICRSToAltAz(icrs)
		if err != nil {
			t.Fatalf("ICRSToAltAz: %v", err)
		}

		stellarRefraction := sr.Alt().Degrees() - sg.Alt().Degrees()

		if d := math.Abs(vectorRefraction-stellarRefraction) * 3600; d > toleranceArcsec {
			t.Errorf("at geometric %+.3f°: the vector pipeline refracts by %.5f° and the "+
				"stellar pipeline by %.5f°, a difference of %.2f arcsec.\n"+
				"  Both must be SOFA's Atioq model — reproduce the routine rather than "+
				"re-deriving it.", geometric, vectorRefraction, stellarRefraction, d)
		}
	}

	if checked < 20 {
		t.Fatalf("only %d directions fell in the -5°..+5° band; the sweep is not "+
			"exercising the horizon", checked)
	}

	t.Logf("%d directions compared across -5°..+5°", checked)
}

// TestRefractionIsPhysicallyBounded asserts what refraction is, independently
// of any other implementation.
//
// Agreement between the two pipelines is necessary and not sufficient: two
// pipelines can be wrong together, and before this change they were wrong
// separately. These bounds hold whatever model is used, so they catch a
// divergence that agreement alone would not.
func TestRefractionIsPhysicallyBounded(t *testing.T) {
	t.Parallel()

	withAtm, noAtm := refractionScene(t)

	// Atmospheric refraction at sea level is about 0.57° at the horizon and
	// falls monotonically to zero at the zenith. Half a degree of headroom
	// admits a plausible model and excludes a diverging one.
	const maxDegrees = 1.0

	type sample struct {
		geometric, refraction float64
	}

	var samples []sample

	for raDeg := 0.0; raDeg < 360.0; raDeg += 0.1 {
		icrs := coord.NewICRS(angle.Deg(raDeg), angle.Deg(-20))
		far := farVector(icrs)

		geometric := noAtm.GeocentricToObserved(far).Alt().Degrees()
		refraction := withAtm.GeocentricToObserved(far).Alt().Degrees() - geometric

		switch {
		case math.IsNaN(refraction) || math.IsInf(refraction, 0):
			t.Fatalf("at geometric %+.3f°: refraction is %v", geometric, refraction)
		case refraction < 0:
			t.Errorf("at geometric %+.3f°: refraction is %.4f°, but refraction raises "+
				"an object, never lowers it", geometric, refraction)
		case refraction > maxDegrees:
			t.Errorf("at geometric %+.3f°: refraction is %.4f°, which exceeds anything "+
				"physical — the series has diverged. This is the +7028° defect's "+
				"signature.", geometric, refraction)
		}

		if geometric > -5 && geometric < 85 {
			samples = append(samples, sample{geometric, refraction})
		}
	}

	if len(samples) < 50 {
		t.Fatalf("only %d usable samples; the sweep is not covering the sky", len(samples))
	}

	// Monotonic in altitude, but only clear of the clamp.
	//
	// SOFA bounds sin(altitude) at 0.05 — about 2.87° — and below that the
	// refraction is not flat: the clamp fixes the vertical component while the
	// horizontal one keeps varying, so tan(z) still moves and refraction drifts
	// slowly. Where the clamp stops binding the two regimes meet in a small
	// kink, and refraction briefly rises with altitude.
	//
	// Measured, that kink extends to 3.097° and is worth about 3 arcsec. It is
	// a property of Atioq's model, not a defect in reproducing it, so the band
	// is excluded rather than asserted away — an earlier version of this test
	// used the nominal 2.87° clamp and failed on the model's own behaviour.
	// 4° clears it with margin.
	const monotonicAbove = 4.0

	for i := range samples {
		for j := range samples {
			a, b := samples[i], samples[j]
			if a.geometric <= monotonicAbove || b.geometric <= a.geometric {
				continue
			}

			// b is higher than a, so it must refract no more than a does.
			if b.refraction > a.refraction+1e-9 {
				t.Errorf("refraction rises with altitude: %.5f° at %+.3f° but %.5f° at "+
					"%+.3f°", a.refraction, a.geometric, b.refraction, b.geometric)

				return
			}
		}
	}

	t.Logf("%d samples bounded; monotonic above %.1f°", len(samples), monotonicAbove)
}
