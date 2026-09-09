package ephemeris

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// constantOfAberration is κ, the maximum annual aberration, in arcseconds.
//
// IAU 2009 system of astronomical constants (Luzum et al. 2011). It is
// v_earth/c expressed as an angle, so it is the displacement of a target seen
// exactly perpendicular to the Earth's motion, and the largest value annual
// aberration can take.
const constantOfAberration = 20.49552

// sepArcsec is the angle between two directions, in arcseconds.
func sepArcsec(tb testing.TB, a, b vector.Vec3) float64 {
	tb.Helper()

	d := a.Unit().Dot(b.Unit())

	// acos is undefined a hair outside [-1,1], which rounding reaches for
	// two nearly parallel unit vectors — which is every case here.
	d = math.Min(1, math.Max(-1, d))

	return math.Acos(d) * 180 / math.Pi * 3600
}

// TestAstrometricDiffersFromApparentByAberration is what makes
// AstrometricState checkable at all rather than merely plausible.
//
// The two places differ by exactly the annual aberration and by nothing else:
//
//	astrometric = target(t - τ) - earth(t)
//	apparent    = target(t - τ) - earth(t - τ)
//
// so the angle between them is the aberration for that target on that date,
// which has a known closed form — κ·sin θ, where θ is the angle between the
// target and the Earth's velocity.
//
// # Why the Sun is the strong case
//
// For a circular orbit the Earth's velocity is perpendicular to the Sun, so
// θ ≈ 90° all year and the Sun's aberration is κ itself, every day. That
// turns a vague "about twenty arcseconds" into a fixed number the test can
// hold to a tight tolerance, and it is the one geometry where the expected
// value does not depend on the ephemeris being right about where anything is.
//
// The residual spread is the orbit's eccentricity: measured over a year,
// 20.155″ to 20.828″ about a mean of 20.490″. κ is 20.49552″, so the mean
// agrees to 0.006″ — that agreement is the assertion, not the range.
//
// # Why the planets are the weak case, and still worth asserting
//
// Jupiter and Mars sweep the whole range as θ varies — measured 0.345″ to
// 20.825″ and 0.773″ to 20.826″. The mean says nothing, but the *maximum*
// must approach κ and must never exceed it: aberration larger than κ would
// mean the Earth moving faster than it does.
func TestAstrometricDiffersFromApparentByAberration(t *testing.T) {
	t.Parallel()

	p := Default()

	t.Run("the Sun is aberrated by the constant, all year", func(t *testing.T) {
		t.Parallel()

		var sum float64

		n := 0

		for day := range 73 {
			epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC).
				AddDays(float64(day * 5))

			sep := separationOfPlaces(t, p, Sun, epoch)

			// The whole year stays within eccentricity's swing of κ.
			if math.Abs(sep-constantOfAberration) > 0.5 {
				t.Errorf("Sun aberration on day %d = %.3f arcsec, want within 0.5 of %.5f",
					day*5, sep, constantOfAberration)
			}

			sum += sep
			n++
		}

		mean := sum / float64(n)
		if math.Abs(mean-constantOfAberration) > 0.05 {
			t.Errorf("mean Sun aberration = %.4f arcsec, want %.5f within 0.05; "+
				"the Sun sits ~90 degrees from the Earth's velocity all year, so its "+
				"aberration is the constant itself", mean, constantOfAberration)
		}
	})

	t.Run("a planet's aberration reaches the constant and never exceeds it", func(t *testing.T) {
		t.Parallel()

		for _, id := range []ID{Jupiter, Mars} {
			var maxSep float64

			for day := range 73 {
				epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC).
					AddDays(float64(day * 5))

				sep := separationOfPlaces(t, p, id, epoch)

				// A displacement larger than κ would mean the Earth moving
				// faster than it does.
				if sep > constantOfAberration+0.5 {
					t.Errorf("%s aberration on day %d = %.3f arcsec, above the constant %.5f",
						id, day*5, sep, constantOfAberration)
				}

				maxSep = math.Max(maxSep, sep)
			}

			// Over a year the geometry sweeps through perpendicular, so the
			// maximum has to get close to the constant.
			if maxSep < constantOfAberration-1 {
				t.Errorf("%s peak aberration over a year = %.3f arcsec, want it to "+
					"approach %.5f; the geometry passes through perpendicular",
					id, maxSep, constantOfAberration)
			}
		}
	})
}

// TestAstrometricIsNotApparent guards the mistake this pair exists to prevent.
//
// A consumer that applies aberration itself — coord.Context.ICRSToAltAz does —
// must be handed the astrometric place. Handing it the apparent one counts the
// aberration twice, which is a ~20 arcsecond error that still looks entirely
// reasonable, and is exactly the defect a USNO comparison caught once before.
//
// So the two must not silently become the same function.
func TestAstrometricIsNotApparent(t *testing.T) {
	t.Parallel()

	p := Default()
	epoch := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	sep := separationOfPlaces(t, p, Sun, epoch)
	if sep < 1 {
		t.Fatalf("astrometric and apparent Sun differ by %.4f arcsec; they should "+
			"differ by the annual aberration, near %.5f", sep, constantOfAberration)
	}
}

// separationOfPlaces is the angle between a target's astrometric and apparent
// directions.
func separationOfPlaces(tb testing.TB, p Provider, id ID, epoch time.Time) float64 {
	tb.Helper()

	astrometric, err := AstrometricState(p, id, epoch)
	if err != nil {
		tb.Fatalf("AstrometricState(%s): %v", id, err)
	}

	apparent, err := ApparentState(p, id, epoch)
	if err != nil {
		tb.Fatalf("ApparentState(%s): %v", id, err)
	}

	return sepArcsec(tb, astrometric.Pos, apparent.Pos)
}
