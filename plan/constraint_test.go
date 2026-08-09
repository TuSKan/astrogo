package plan

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/time"
)

func TestConstraints(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	// Equinox 2000
	tm := time.FromJD(2451545.0, time.UTC)

	t.Run("Altitude", func(t *testing.T) {
		c := Altitude{Threshold: angle.Deg(20)}
		// Target near zenith at Greenwich
		obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))
		res, err := c.Check(obj, tm, site)
		testutil.AssertNoError(t, err)

		if !res.Pass {
			t.Errorf("Expected PASS for high altitude, got %v", res)
		}

		c2 := Altitude{Threshold: angle.Deg(95)}

		res2, _ := c2.Check(obj, tm, site)
		if res2.Pass {
			t.Errorf("Expected FAIL for extreme threshold, got %v", res2)
		}
	})

	t.Run("Airmass", func(t *testing.T) {
		c := Airmass{Threshold: 2.0}
		obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))
		res, err := c.Check(obj, tm, site)
		testutil.AssertNoError(t, err)

		if !res.Pass {
			t.Errorf("Expected PASS for low airmass, got %v", res)
		}

		// Below horizon target
		obj2 := NewStar("T", angle.Deg(0), angle.Deg(45))
		res2, err := c.Check(obj2, tm, site)
		testutil.AssertNoError(t, err)

		if res2.Pass {
			t.Error("Expected FAIL for target below horizon")
		}
	})
}

func TestSunMoonConstraints(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)

	// Night time (Sun below horizon)
	tmNight := time.FromJD(2451545.5, time.UTC)

	t.Run("Sun", func(t *testing.T) {
		c := Sun{Threshold: angle.Deg(-12)}
		res, err := c.Check(nil, tmNight, site)
		testutil.AssertNoError(t, err)

		if !res.Pass {
			t.Errorf("Expected PASS during night, got %v", res)
		}
	})

	t.Run("MoonSep", func(t *testing.T) {
		c := MoonSep{Threshold: angle.Deg(30)}
		// Target at (0,0)
		obj := NewStar("T", angle.Deg(0), angle.Deg(0))
		res, err := c.Check(obj, tmNight, site)
		testutil.AssertNoError(t, err)
		// Moon position at tmNight is roughly RA=19h, Dec=-16deg.
		// Separation should be > 30 deg from (0,0).
		if !res.Pass {
			t.Errorf("Expected PASS for far moon, got %v", res)
		}
	})

	// Regression: MoonSep.CheckCtx previously had a signature
	// (obj, ctx) that didn't match the ConstraintCtx interface
	// (obj, t, site, ctx), so it silently never satisfied ConstraintCtx
	// and never got the hot-path Context-reuse benefit.
	t.Run("MoonSep satisfies ConstraintCtx", func(t *testing.T) {
		var c ConstraintCtx = MoonSep{Threshold: angle.Deg(30)}

		obj := NewStar("T", angle.Deg(0), angle.Deg(0))
		ctx := coord.NewContext(tmNight, site.Location(), site.Refraction())

		want, err := MoonSep{Threshold: angle.Deg(30)}.Check(obj, tmNight, site)
		testutil.AssertNoError(t, err)

		got, err := c.CheckCtx(obj, tmNight, site, ctx)
		testutil.AssertNoError(t, err)

		if got.Pass != want.Pass || got.Value != want.Value {
			t.Errorf("CheckCtx result %+v does not match Check result %+v", got, want)
		}
	})

	t.Run("MoonIllum passes below threshold", func(t *testing.T) {
		obj := NewStar("T", angle.Deg(0), angle.Deg(0))

		frac, _, err := MoonIllumination(tmNight, eph.Default())
		testutil.AssertNoError(t, err)

		c := MoonIllum{Threshold: frac + 0.1} // comfortably above the real value
		res, err := c.Check(obj, tmNight, site)
		testutil.AssertNoError(t, err)

		if !res.Pass {
			t.Errorf("expected PASS for a threshold above the real illumination, got %v", res)
		}

		if res.Value != frac {
			t.Errorf("Result.Value = %v, want %v (the real illumination fraction)", res.Value, frac)
		}
	})

	t.Run("MoonIllum fails above threshold", func(t *testing.T) {
		obj := NewStar("T", angle.Deg(0), angle.Deg(0))

		frac, _, err := MoonIllumination(tmNight, eph.Default())
		testutil.AssertNoError(t, err)

		c := MoonIllum{Threshold: frac - 0.1} // comfortably below the real value
		if c.Threshold < 0 {
			t.Skip("test setup: real illumination too low to construct a valid sub-zero threshold")
		}

		res, err := c.Check(obj, tmNight, site)
		testutil.AssertNoError(t, err)

		if res.Pass {
			t.Errorf("expected FAIL for a threshold below the real illumination, got %v", res)
		}

		if res.Reason == "" {
			t.Error("expected a non-empty Reason on failure")
		}
	})

	t.Run("MoonIllum always passes for the Moon itself", func(t *testing.T) {
		moon := NewMoon(eph.Default())

		c := MoonIllum{Threshold: 0} // impossible to satisfy on illumination alone
		res, err := c.Check(moon, tmNight, site)
		testutil.AssertNoError(t, err)

		if !res.Pass {
			t.Errorf("expected PASS when observing the Moon itself regardless of threshold, got %v", res)
		}
	})

	// Regression: mirrors the MoonSep ConstraintCtx signature-drift
	// regression above -- CheckCtx must match ConstraintCtx exactly.
	t.Run("MoonIllum satisfies ConstraintCtx", func(t *testing.T) {
		var c ConstraintCtx = MoonIllum{Threshold: 0.5}

		obj := NewStar("T", angle.Deg(0), angle.Deg(0))
		ctx := coord.NewContext(tmNight, site.Location(), site.Refraction())

		want, err := MoonIllum{Threshold: 0.5}.Check(obj, tmNight, site)
		testutil.AssertNoError(t, err)

		got, err := c.CheckCtx(obj, tmNight, site, ctx)
		testutil.AssertNoError(t, err)

		if got.Pass != want.Pass || got.Value != want.Value {
			t.Errorf("CheckCtx result %+v does not match Check result %+v", got, want)
		}
	})

	t.Run("MoonIllum nil Provider defaults to eph.Default()", func(t *testing.T) {
		obj := NewStar("T", angle.Deg(0), angle.Deg(0))

		withNil, err := MoonIllum{Threshold: 1}.Check(obj, tmNight, site)
		testutil.AssertNoError(t, err)

		withDefault, err := MoonIllum{Threshold: 1, Provider: eph.Default()}.Check(obj, tmNight, site)
		testutil.AssertNoError(t, err)

		if withNil.Value != withDefault.Value {
			t.Errorf("nil Provider gave %v, eph.Default() gave %v -- should be identical", withNil.Value, withDefault.Value)
		}
	})

	t.Run("MoonIllum propagates a provider error", func(t *testing.T) {
		obj := NewStar("T", angle.Deg(0), angle.Deg(0))

		c := MoonIllum{Threshold: 0.5, Provider: errStateProvider{}}

		_, err := c.Check(obj, tmNight, site)
		if !errors.Is(err, errMoonPhaseTestProvider) {
			t.Errorf("Check error = %v, want it to wrap errMoonPhaseTestProvider", err)
		}
	})
}
