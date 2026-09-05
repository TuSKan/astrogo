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

// paranalSite is the reference observatory used by the tests in this file.
func paranalSite(t *testing.T) *coord.Geodetic {
	t.Helper()

	site, err := coord.NewGeodetic(angle.Deg(-70.4028), angle.Deg(-24.6251), 2635)
	if err != nil {
		t.Fatalf("site: %v", err)
	}

	return site
}

// seaLevelAir is a standard refraction environment with no model set.
func seaLevelAir() atmosphere.Refraction {
	return atmosphere.Refraction{
		Pressure: 1013.25, Temperature: 15.0, Humidity: 0.5, Wavelength: 0.55,
	}
}

// TestNilModelMatchesExplicitSOFAModel pins the one duplication this design
// accepts on purpose.
//
// GeocentricToObserved does not call atmosphere at all when Model is nil: it
// applies SOFA's series through refractLikeAtioq, using the Refa and Refb
// constants Apco13 already cached for the epoch. That is a real performance
// decision — recomputing them per call would cost a Refco on a path measured
// in hundreds of nanoseconds.
//
// So the same model exists twice: once as cached constants inside coord, once
// as atmosphere.RefractionSOFA, which recomputes them and is what a nil Model
// resolves to everywhere else. Two copies of one model can drift, and nothing
// would say so — which is what this test is for.
//
// Measured agreement over 2000 above-horizon directions: 0.012 arcsec above 3
// degrees, 0.7 milliarcsecond above 5 degrees. The residual is the difference
// between rotating a direction vector, as SOFA's Atioq does, and adding a
// scalar correction to an altitude; it shrinks as refraction does. Below about
// 3 degrees both are inside SOFA's own sin(altitude) clamp, where refraction is
// not a trustworthy quantity in any case.
func TestNilModelMatchesExplicitSOFAModel(t *testing.T) {
	when := time.Date(2026, time.June, 15, 22, 0, 0, 0, time.LocationUTC)
	site := paranalSite(t)

	nilEnv := seaLevelAir()

	explicit := nilEnv
	explicit.Model = atmosphere.RefractionSOFA{}

	ctxNil := coord.NewContext(when, site, nilEnv)
	ctxExplicit := coord.NewContext(when, site, explicit)

	// Contract per altitude floor, in arcseconds.
	bounds := []struct {
		floorDeg float64
		maxDiff  float64
	}{
		{3, 0.05},
		{5, 0.005},
		{10, 0.001},
	}

	worst := make([]float64, len(bounds))
	above := 0

	// A deterministic spiral over the sphere, dense enough to find the worst case.
	for i := range 4000 {
		th := float64(i) * 0.7853981633974483
		ph := float64(i) * 0.24

		v := vector.Vec3{
			X: math.Cos(ph) * math.Cos(th),
			Y: math.Cos(ph) * math.Sin(th),
			Z: math.Sin(ph),
		}.MulScalar(1e10) // far enough that parallax is not in play

		a := ctxNil.GeocentricToObserved(v)
		b := ctxExplicit.GeocentricToObserved(v)

		alt := a.Alt().Degrees()
		if alt < 0 {
			continue
		}

		above++

		diff := math.Abs((a.Alt() - b.Alt()).Arcseconds())

		for j, bound := range bounds {
			if alt >= bound.floorDeg && diff > worst[j] {
				worst[j] = diff
			}
		}

		// Azimuth is untouched by refraction and must be bit-identical.
		if az := math.Abs((a.Az() - b.Az()).Arcseconds()); az != 0 {
			t.Fatalf("azimuth differs by %g arcsec at %.2f deg altitude; refraction "+
				"must not move it", az, alt)
		}
	}

	if above < 500 {
		t.Fatalf("only %d of 4000 sample directions were above the horizon; the "+
			"sampling no longer covers the range under test", above)
	}

	for j, bound := range bounds {
		if worst[j] > bound.maxDiff {
			t.Errorf("above %.0f deg: the cached-constant path and "+
				"atmosphere.RefractionSOFA differ by %.6f arcsec, contract %.3f.\n"+
				"  These are two copies of one model; they have drifted apart.",
				bound.floorDeg, worst[j], bound.maxDiff)
		}
	}

	t.Logf("%d above-horizon samples; worst difference %.6f arcsec above 3 deg, "+
		"%.6f above 5 deg, %.6f above 10 deg", above, worst[0], worst[1], worst[2])
}

// TestDisperseReportsDispersionWhereReduceAppliedRefraction is the second half
// of the nil-model defect.
//
// Disperse used to check Model == nil and, finding it, return the observed
// position unchanged for every wavelength — zero dispersion. But Reduce, on
// the very same Reducer, had just refracted through SOFA, because that is what
// a nil model with a pressure means. One reduction, two answers about whether
// there is an atmosphere.
func TestDisperseReportsDispersionWhereReduceAppliedRefraction(t *testing.T) {
	when := time.Date(2026, time.June, 15, 22, 0, 0, 0, time.LocationUTC)
	site := paranalSite(t)

	env := seaLevelAir()
	if env.Model != nil {
		t.Fatal("precondition: this test is about the nil-Model path")
	}

	r := coord.NewReducer(site, when, env)

	// A direction well above the horizon, so refraction is real but modest.
	var target vector.Vec3

	for i := range 4000 {
		th := float64(i) * 0.7853981633974483
		ph := float64(i) * 0.24
		v := vector.Vec3{
			X: math.Cos(ph) * math.Cos(th),
			Y: math.Cos(ph) * math.Sin(th),
			Z: math.Sin(ph),
		}.MulScalar(1e10)

		if alt := r.Reduce(v).Observed.Alt().Degrees(); alt > 25 && alt < 50 {
			target = v
			break
		}
	}

	if target.Norm() == 0 {
		t.Fatal("no sample direction landed between 25 and 50 degrees altitude")
	}

	res := r.Disperse(target, []float64{0.40, 0.55, 0.70})

	if len(res.Dispersion) != 3 {
		t.Fatalf("got %d dispersion entries, want 3", len(res.Dispersion))
	}

	blue, red := res.Dispersion[0.40], res.Dispersion[0.70]

	// Reduce refracted, so Disperse must too.
	if lift := (res.Dispersion[0.55].Alt() - res.Geometric.Alt()).Arcseconds(); lift < 20 {
		t.Errorf("the dispersed position at 0.55 um sits %.3f arcsec above the "+
			"geometric one, but Reduce refracted this direction by a comparable "+
			"amount. Disperse is reporting an atmosphere that is not there.", lift)
	}

	spread := (blue.Alt() - red.Alt()).Arcseconds()
	if spread <= 0 {
		t.Fatalf("blue sits %.4f arcsec below red; blue must refract more", -spread)
	}

	if spread < 0.5 || spread > 6 {
		t.Errorf("dispersion between 0.40 and 0.70 um = %.3f arcsec, want roughly "+
			"0.5-6 arcsec at this altitude", spread)
	}

	// Azimuth is unaffected by refraction, at any wavelength.
	if d := (blue.Az() - red.Az()).Arcseconds(); math.Abs(d) > 1e-9 {
		t.Errorf("azimuth varies with wavelength by %g arcsec", d)
	}
}

// TestDisperseStillReportsNoDispersionInAVacuum pins the other half of the
// resolution: with no pressure there is no atmosphere, so every wavelength
// lands on the geometric position and agrees with what Reduce observed.
func TestDisperseStillReportsNoDispersionInAVacuum(t *testing.T) {
	when := time.Date(2026, time.June, 15, 22, 0, 0, 0, time.LocationUTC)
	site := paranalSite(t)

	r := coord.NewReducer(site, when, atmosphere.Refraction{})

	v := vector.Vec3{X: 0.3, Y: 0.5, Z: 0.81}.MulScalar(1e10)
	res := r.Disperse(v, []float64{0.40, 0.70})

	for wl, got := range res.Dispersion {
		if d := (got.Alt() - res.Geometric.Alt()).Arcseconds(); math.Abs(d) > 1e-9 {
			t.Errorf("at %.2f um the dispersed altitude is %g arcsec from the "+
				"geometric one, but there is no atmosphere", wl, d)
		}

		if d := (got.Alt() - res.Observed.Alt()).Arcseconds(); math.Abs(d) > 1e-9 {
			t.Errorf("at %.2f um Disperse and Reduce disagree by %g arcsec", wl, d)
		}
	}
}
