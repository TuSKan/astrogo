package satellite_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestAltitudeAgreesWithTheEarthFixedRoute holds Altitude, which reads the
// height off the TEME position with no rotation, to the rigorous route: State
// into GCRS, Context.ICRSToITRS into the Earth-fixed frame, and the height of
// that point above WGS84.
//
// The two can only agree if TEME and ITRS share their z axis, which is the
// claim Altitude's shortcut rests on (#512). They do up to polar motion, which
// is zero here because no EOP are loaded, so what is left is float rounding
// across two different chains of rotations. Measured, the largest difference
// over a day of ISS positions is well under a micrometer; the bound is a
// millimeter, which is still four orders below anything that would mean a
// frame disagreement.
func TestAltitudeAgreesWithTheEarthFixedRoute(t *testing.T) {
	t.Parallel()

	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	site, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	metersPerAU := constants.IAU.AstronomicalUnit.Value

	start := time.Date(2026, 4, 19, 0, 0, 0, 0, time.LocationUTC)

	var worst float64

	for step := range 97 { // every 15 minutes for a day
		tm := start.Add(unit.Minutes(15 * float64(step)))

		got, err := sat.Altitude(tm)
		if err != nil {
			t.Fatalf("Altitude at %v: %v", tm, err)
		}

		st, err := sat.State(0, tm)
		if err != nil {
			t.Fatalf("State at %v: %v", tm, err)
		}

		ctx := coord.NewContext(tm, site, atmosphere.Refraction{Pressure: 0})

		geo, err := coord.FromECEF(ctx.ICRSToITRS(st.Pos.MulScalar(metersPerAU)), coord.WGS84())
		if err != nil {
			t.Fatalf("FromECEF at %v: %v", tm, err)
		}

		d := math.Abs(got.Meters() - geo.Height().Meters())
		worst = max(worst, d)

		if d > 1e-3 {
			t.Errorf("at %v: Altitude %.6f m, the Earth-fixed route %.6f m, %.3g m apart",
				tm, got.Meters(), geo.Height().Meters(), d)
		}
	}

	t.Logf("worst difference over a day: %.3g m", worst)
}

// BenchmarkAltitude measures what Altitude costs per call at a realistic,
// fixed epoch.
func BenchmarkAltitude(b *testing.B) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		b.Fatalf("NewFromTLE: %v", err)
	}

	tm := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)

	for b.Loop() {
		if _, err := sat.Altitude(tm); err != nil {
			b.Fatal(err)
		}
	}
}
