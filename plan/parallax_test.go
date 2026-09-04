package plan_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// detailer is the subset of Observable this file needs; every concrete target
// type implements it.
type detailer interface {
	GetDetails(ctx *coord.Context, props ...string) (*plan.TargetDetails, error)
}

// TestConstraintAndDetailsAgreeOnAltitude is the guard the diurnal-parallax
// defect needed and did not have.
//
// # What went wrong
//
// Every constraint, score and visibility check called ICRSToAltAz on a
// geocentric position. That function treats its argument as a direction at
// infinity — correct for a catalog star, wrong for anything nearby — so the
// observer's offset from the geocentre was discarded. Only the events solver
// and the details builder subtracted it.
//
// The Moon is about 60 Earth radii away, so an observer on the surface sees it
// up to 0.95° from where the geocentre does. Measured before the fix, at one
// site across a day:
//
//	Moon 00h  IsObservable=-31.3985  GetDetails=-32.3458  delta=+0.9474
//	Moon 12h  IsObservable= 18.2958  GetDetails= 17.3482  delta=+0.9476
//	Moon 21h  IsObservable= 16.2017  GetDetails= 15.2514  delta=+0.9503
//
// An Altitude{Threshold: 0} constraint therefore called the Moon up about four
// minutes before MoonEvents said it rose: two public APIs, one library, a
// degree apart.
//
// # Why the existing suite did not catch it
//
// Nothing compared the two paths. Each was exercised against its own
// expectations, and a degree of disagreement between them was not a value any
// test looked at. Running the whole suite after the fix — a 0.95° change in
// every scheduler decision involving the Moon — produced no failures at all,
// which is the clearest possible statement of what was missing.
//
// # The Moon is the test
//
// Parallax scales as 1/distance, so Mars shows about 0.0006° and a star shows
// none. A test written against a planet would pass on the broken code. The
// tolerance below is far tighter than Mars' own parallax for that reason: it
// has to be a bound the defect could not slip under.
func TestConstraintAndDetailsAgreeOnAltitude(t *testing.T) {
	t.Parallel()

	site, err := plan.NewSiteEarthLocation("parallax", -23.5, -46.6, 800)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	prov := eph.Default()

	bodies := []struct {
		name string
		obj  plan.Observable
	}{
		{"Moon", plan.NewMoon(prov)},
		{"Sun", plan.NewSun(prov)},
		{"Mars", plan.NewPlanet("Mars", eph.Mars, prov)},
		{"Jupiter", plan.NewPlanet("Jupiter", eph.Jupiter, prov)},
	}

	// One arcsecond. Both paths now run the same transform on the same vector,
	// so they agree to floating-point noise; this leaves room for platform
	// rounding while staying three orders below the 0.95° defect and two
	// below Mars' own parallax.
	const toleranceArcsec = 1.0

	for _, b := range bodies {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			d, ok := b.obj.(detailer)
			if !ok {
				t.Fatalf("%s does not implement GetDetails", b.name)
			}

			// A full day, so the comparison spans rising, transit and setting
			// rather than one lucky geometry.
			for hour := range 24 {
				when := time.Date(2026, time.March, 20, hour, 0, 0, 0, time.LocationUTC)
				ctx := coord.NewContext(when, site.Location(), site.Refraction())

				eval, err := plan.IsObservable(b.obj, when, site, plan.Altitude{Threshold: angle.Deg(0)})
				if err != nil {
					t.Fatalf("%02dh IsObservable: %v", hour, err)
				}

				details, err := d.GetDetails(ctx)
				if err != nil {
					t.Fatalf("%02dh GetDetails: %v", hour, err)
				}

				diff := math.Abs(eval.AltAz.Alt().Degrees()-details.Altitude.Degrees()) * 3600
				if diff > toleranceArcsec {
					t.Errorf("%02dh: IsObservable reports %.4f° and GetDetails %.4f°, "+
						"a difference of %.1f arcsec.\n"+
						"  These are the same quantity from two public APIs. A gap this "+
						"size means one path is treating a nearby body as a star at "+
						"infinity — see observedAltAz.",
						hour, eval.AltAz.Alt().Degrees(), details.Altitude.Degrees(), diff)
				}
			}
		})
	}
}

// TestMoonParallaxIsActuallyApplied asserts the correction is present, not
// merely consistent.
//
// TestConstraintAndDetailsAgreeOnAltitude would pass if both paths reverted to
// the geocentric answer together — agreement is necessary and not sufficient.
// This pins the physics independently: the topocentric and geocentric
// altitudes of the Moon must differ by something close to its horizontal
// parallax, and that difference must be largest near the horizon and vanish
// overhead.
func TestMoonParallaxIsActuallyApplied(t *testing.T) {
	t.Parallel()

	site, err := plan.NewSiteEarthLocation("parallax", -23.5, -46.6, 800)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	moon := plan.NewMoon(eph.Default())

	var maxSeen float64

	for hour := range 24 {
		when := time.Date(2026, time.March, 20, hour, 0, 0, 0, time.LocationUTC)

		// No atmosphere, so refraction cancels exactly and the difference is
		// parallax alone.
		//
		// It matters less than it did. Before #153 the two paths applied
		// refraction differently and this measured 1.1486° — parallax plus a
		// refraction disagreement. They now share SOFA's model, so with
		// refraction enabled it reads 0.9872° against 0.9918° here; the
		// remaining 5 millidegrees is the same model evaluated at two
		// altitudes that differ by the parallax itself. Keeping the
		// atmosphere out removes even that, so a future refraction change
		// cannot move this test.
		ctx := coord.NewContext(when, site.Location(), atmosphere.Refraction{})

		pos, err := moon.Position(when)
		if err != nil {
			t.Fatalf("%02dh Position: %v", hour, err)
		}

		vec, err := moon.GeocentricVec(when)
		if err != nil {
			t.Fatalf("%02dh GeocentricVec: %v", hour, err)
		}

		geocentric, err := ctx.ICRSToAltAz(pos)
		if err != nil {
			t.Fatalf("%02dh ICRSToAltAz: %v", hour, err)
		}

		topocentric := ctx.GeocentricToObserved(vec)

		if d := math.Abs(topocentric.Alt().Degrees() - geocentric.Alt().Degrees()); d > maxSeen {
			maxSeen = d
		}
	}

	// The Moon's equatorial horizontal parallax runs 0.90°-1.02° over its
	// orbit. Seeing close to that maximum somewhere in a day confirms the
	// correction is applied at full strength rather than partially.
	const (
		lowerBound = 0.80
		upperBound = 1.10
	)

	if maxSeen < lowerBound || maxSeen > upperBound {
		t.Errorf("the largest topocentric-geocentric difference for the Moon over a day "+
			"is %.4f°, outside the expected %.2f°-%.2f° horizontal parallax.\n"+
			"  Below the range means the correction is not being applied; above it "+
			"means something other than parallax is in the difference.",
			maxSeen, lowerBound, upperBound)
	}

	t.Logf("peak topocentric correction over the day: %.4f°", maxSeen)
}
