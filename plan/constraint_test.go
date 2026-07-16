package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/time"
)

func TestConstraints(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc, nil)
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
	site, _ := NewSite("Test", loc, nil)

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
		ctx := coord.NewContext(tmNight, site.Location(), site.Atmosphere())

		want, err := MoonSep{Threshold: angle.Deg(30)}.Check(obj, tmNight, site)
		testutil.AssertNoError(t, err)

		got, err := c.CheckCtx(obj, tmNight, site, ctx)
		testutil.AssertNoError(t, err)

		if got.Pass != want.Pass || got.Value != want.Value {
			t.Errorf("CheckCtx result %+v does not match Check result %+v", got, want)
		}
	})
}
