package coord

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// readEverything calls every method of ctx that reaches SOFA through its
// astrometry: the stellar path above and below 10° of altitude, where the
// place is taken again with refraction off, the hour angle, the vector path
// and the inverse, at altitudes either side of the same 10°.
func readEverything(ctx *Context) {
	for ra := 0.0; ra < 360; ra += 30 {
		for dec := -80.0; dec <= 80; dec += 20 {
			c := NewICRS(angle.Deg(ra), angle.Deg(dec))

			_, _ = ctx.ICRSToAltAz(c)
			_, _ = ctx.ICRSToHourAngle(c)
			ctx.CIRSToObserved(ctx.AstrometricToCIRS(c.Astrometric()))
			ctx.GeocentricToObserved(c.ToUnitVector().MulScalar(0.0025))
		}
	}

	for _, alt := range []float64{-1, 3, 9, 11, 45, 85} {
		for _, az := range []float64{0, 135, 270} {
			_, _ = ctx.AltAzToICRS(NewAltAz(angle.Deg(alt), angle.Deg(az)))
		}
	}
}

// readOnlyContexts are a Context SOFA refracts for, one with no atmosphere,
// and one with an explicit refraction model, which refracts outside SOFA.
func readOnlyContexts(t *testing.T) []*Context {
	t.Helper()

	site, err := NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	at := time.Date(2026, time.March, 20, 3, 0, 0, 0, time.LocationUTC)

	modeled := atmosphere.StandardRefraction()
	modeled.Model = atmosphere.RefractionBennett{}

	return []*Context{
		NewContext(at, site, atmosphere.StandardRefraction()),
		NewContext(at, site, atmosphere.Refraction{}),
		NewContext(at, site, modeled),
	}
}

// TestReadsLeaveTheAstrometryUntouched is the claim the package's Concurrency
// section and the shared Context in the batch methods rest on: reading a
// Context writes nothing. Until #675 the batch methods cloned a Context per
// worker, on the belief that SOFA's iauAtioq caches refraction coefficients
// in the astrometry. In gofa v1.19.1 neither it nor Atciq, Atoiq or Aticq
// writes to it; this holds the whole Context to that, should a later gofa
// start to.
func TestReadsLeaveTheAstrometryUntouched(t *testing.T) {
	t.Parallel()

	for i, ctx := range readOnlyContexts(t) {
		before := *ctx

		readEverything(ctx)

		if ctx.astrom != before.astrom {
			t.Errorf("context %d: reading it changed its astrometry:\n before %+v\n after  %+v", i, before.astrom, ctx.astrom)
		}

		if ctx.mat != before.mat || ctx.obsVec != before.obsVec || ctx.diurab != before.diurab || ctx.eo != before.eo {
			t.Errorf("context %d: reading it changed its cached reduction", i)
		}
	}
}

// TestOneContextServesConcurrentReaders shares each Context across goroutines
// running every read at once, as the batch methods do. Under -race, which CI
// runs, a write anywhere on these paths is a reported race; without it, every
// goroutine must still get the serial answer.
func TestOneContextServesConcurrentReaders(t *testing.T) {
	t.Parallel()

	star := NewICRS(angle.Hour(5.5), angle.Deg(-5.4))
	low := NewAltAz(angle.Deg(3), angle.Deg(135))
	vec := vector.Vec3{X: 0.0016, Y: 0.0019, Z: 0.0008}

	for i, ctx := range readOnlyContexts(t) {
		wantAA, _ := ctx.ICRSToAltAz(star)
		wantBack, _ := ctx.AltAzToICRS(low)
		wantGeo := ctx.GeocentricToObserved(vec)

		const readers = 8

		errs := make(chan string, readers)

		for range readers {
			go func() {
				readEverything(ctx)

				aa, _ := ctx.ICRSToAltAz(star)
				back, _ := ctx.AltAzToICRS(low)
				geo := ctx.GeocentricToObserved(vec)

				switch {
				case aa.Alt() != wantAA.Alt() || aa.Az() != wantAA.Az():
					errs <- "ICRSToAltAz"
				case back.RA() != wantBack.RA() || back.Dec() != wantBack.Dec():
					errs <- "AltAzToICRS"
				case geo.Alt() != wantGeo.Alt() || geo.Az() != wantGeo.Az():
					errs <- "GeocentricToObserved"
				default:
					errs <- ""
				}
			}()
		}

		for range readers {
			if what := <-errs; what != "" {
				t.Errorf("context %d: a concurrent reader's %s differs from the serial answer", i, what)
			}
		}
	}
}
