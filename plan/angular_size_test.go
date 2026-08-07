package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// fixedVecProvider is a deterministic eph.Provider returning the same
// geocentric position vector (in AU) for any body ID — enough control to
// place a body at an exact, known topocentric distance for AngularDiameter's
// known-value fixtures below, without needing a real ephemeris.
type fixedVecProvider struct {
	vec vector.Vec3
}

func (p *fixedVecProvider) State(_ eph.ID, _ time.Time) (eph.State, error) {
	return eph.State{Pos: p.vec}, nil
}

func (p *fixedVecProvider) Close() error { return nil }

// TestAngularDiameter_KnownValues cross-checks AngularDiameter against
// independently well-known reference angular sizes (not re-derived from
// this package's own radius table, to avoid a tautological test): the
// Sun's angular diameter at 1 AU (~1919.3″, the textbook/almanac figure),
// the Moon's at its mean distance (~31.1′), and Jupiter's near a typical
// opposition-adjacent distance (~47″). Each reference figure is quoted to
// only 3-4 significant figures, so tolerances are set accordingly (looser
// for the Moon, whose ~31.1′ figure implies ±0.05′ ≈ ±3″ of its own
// rounding) — not tightened to this package's own computed precision.
//
// The target vector is built as ctx.ObsVec() + a fixed offset, not a bare
// vector.Vec3{X: distanceAU}, so the resulting TOPOCENTRIC distance
// (vec - ctx.ObsVec(), what AngularDiameter actually measures) is exactly
// the intended distanceAU regardless of the test observer's own position —
// otherwise the ~6378 km observer-to-geocenter offset (testContext's site
// sits on Earth's surface, not at the geocenter) would swing the Moon
// case by several arcseconds depending on the fixed JD's sidereal time,
// since that offset is a non-negligible fraction of the Moon's distance
// (though negligible at the Sun's/Jupiter's AU-scale distances).
func TestAngularDiameter_KnownValues(t *testing.T) {
	ctx := testContext(t)
	tm := ctx.Time()

	cases := []struct {
		name        string
		id          eph.ID
		distanceAU  float64
		wantArcsec  float64
		toleranceAS float64
	}{
		{"Sun at 1 AU", eph.Sun, 1.0, 1919.3, 0.5},
		{"Moon at mean distance", eph.Moon, 384_400_000.0 / 1.495978707e11, 31.1 * 60, 5.0},
		{"Jupiter at 4.2 AU", eph.Jupiter, 4.2, 47.0, 0.5},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prov := &fixedVecProvider{vec: ctx.ObsVec().Add(vector.Vec3{X: c.distanceAU})}
			p := NewPlanet(c.name, c.id, prov)

			diam, err := AngularDiameter(p, tm, ctx)
			if err != nil {
				t.Fatalf("AngularDiameter: %v", err)
			}

			gotArcsec := diam.Arcseconds()
			if math.Abs(gotArcsec-c.wantArcsec) > c.toleranceAS {
				t.Errorf("AngularDiameter = %.2f″, want %.2f″ ± %.2f″", gotArcsec, c.wantArcsec, c.toleranceAS)
			}

			// Convenience method on *Planet must agree exactly with the
			// package-level function it wraps.
			diam2, err := p.AngularDiameter(tm, ctx)
			if err != nil {
				t.Fatalf("(*Planet).AngularDiameter: %v", err)
			}

			if diam2 != diam {
				t.Errorf("(*Planet).AngularDiameter = %v, want it to match AngularDiameter = %v", diam2, diam)
			}
		})
	}
}

// TestAngularDiameter_ErrNoPhysicalRadius confirms a body with no entry in
// the equatorial-radius table (e.g. an asteroid) fails with the documented
// sentinel rather than silently returning a bogus size.
func TestAngularDiameter_ErrNoPhysicalRadius(t *testing.T) {
	ctx := testContext(t)
	tm := ctx.Time()

	const asteroidID eph.ID = 2000099 // arbitrary SPK-style ID, not in bodyEquatorialRadiusM

	prov := &fixedVecProvider{vec: vector.Vec3{X: 1.5}}
	a := NewGenericBody("Test Asteroid", asteroidID, prov)

	_, err := AngularDiameter(a, tm, ctx)
	if !errors.Is(err, ErrNoPhysicalRadius) {
		t.Fatalf("AngularDiameter error = %v, want ErrNoPhysicalRadius", err)
	}
}

// TestAngularDiameter_AsteroidPhysicalRadiusFallback confirms AngularDiameter
// now succeeds for an *Asteroid with WithAlbedo/WithDiameter set, instead
// of always failing with ErrNoPhysicalRadius the way any Asteroid did
// before AngularDiameter fell back to the PhysicalRadius optional
// capability.
func TestAngularDiameter_AsteroidPhysicalRadiusFallback(t *testing.T) {
	ctx := testContext(t)
	tm := ctx.Time()

	const asteroidID eph.ID = 2000433 // Eros, not in bodyEquatorialRadiusM

	// 1 AU topocentric distance, matching TestAngularDiameter_KnownValues'
	// own convention for an easy-to-check angle.
	prov := &fixedVecProvider{vec: ctx.ObsVec().Add(vector.Vec3{X: 1.0})}
	a := NewAsteroid("Eros", asteroidID, prov, WithHG(10.40, 0.46), WithAlbedo(0.25))

	got, err := AngularDiameter(a, tm, ctx)
	if err != nil {
		t.Fatalf("AngularDiameter: %v (expected success via PhysicalRadius fallback)", err)
	}

	if got.Arcseconds() <= 0 {
		t.Errorf("AngularDiameter = %v, want a positive angle", got)
	}
}

// TestAngularDiameter_ZeroDistance confirms the divide-by-zero guard: a
// body placed exactly at the observer's own ICRS position (zero
// topocentric distance) fails with ErrZeroDistance instead of propagating
// a NaN/Inf angle — see plan/solver.go's item-6 fix for why silent
// non-finite propagation is worth guarding against explicitly.
func TestAngularDiameter_ZeroDistance(t *testing.T) {
	ctx := testContext(t)
	tm := ctx.Time()

	// Placing the body exactly at the observer's own ICRS position makes
	// the topocentric vector (body - observer) exactly zero.
	prov := &fixedVecProvider{vec: ctx.ObsVec()}
	sun := NewSun(prov)

	_, err := AngularDiameter(sun, tm, ctx)
	if !errors.Is(err, ErrZeroDistance) {
		t.Fatalf("AngularDiameter error = %v, want ErrZeroDistance", err)
	}
}

// TestAngularDiameter_DetailsAutoPopulateAndOverride confirms the
// plan/details.go wiring: TargetDetails.AngularSize is populated
// automatically for a MovingBody with a known radius, but a caller-supplied
// "AngularSize" prop (applied after fillMovingBody, per computeDetails'
// ordering) still wins.
func TestAngularDiameter_DetailsAutoPopulateAndOverride(t *testing.T) {
	ctx := testContext(t)

	prov := &fixedVecProvider{vec: vector.Vec3{X: 1.0}}
	sun := NewSun(prov)

	d, err := sun.GetDetails(ctx)
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.AngularSize == "" {
		t.Error("GetDetails: AngularSize was not auto-populated for the Sun")
	}

	dOverride, err := sun.GetDetails(ctx, "AngularSize", "OVERRIDDEN")
	if err != nil {
		t.Fatalf("GetDetails with override: %v", err)
	}

	if dOverride.AngularSize != "OVERRIDDEN" {
		t.Errorf("GetDetails with override: AngularSize = %q, want %q", dOverride.AngularSize, "OVERRIDDEN")
	}
}

// TestFormatAngularSize confirms the arcminute/arcsecond crossover at
// exactly 1 arcminute.
func TestFormatAngularSize(t *testing.T) {
	cases := []struct {
		name string
		a    func() string
		want string
	}{
		{"above 1 arcmin", func() string { return formatAngularSize(angle.Arcmin(31.1)) }, "31.10′"},
		{"below 1 arcmin", func() string { return formatAngularSize(angle.Arcsec(15.42)) }, "15.42″"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a(); got != c.want {
				t.Errorf("formatAngularSize = %q, want %q", got, c.want)
			}
		})
	}
}
