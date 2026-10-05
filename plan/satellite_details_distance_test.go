package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// TestSatelliteDetailsDistanceIsTheRange is #495: GetDetails reported every
// satellite at 0 km, having replaced the distance it computed with that of an
// AltAz that carried none. It must be the range LookAngle reports.
func TestSatelliteDetailsDistanceIsTheRange(t *testing.T) {
	t.Parallel()

	prov, site := testISS(t)
	sat := NewSatellite("ISS", 0, prov)

	for _, at := range []time.Time{
		time.Date(2026, time.April, 17, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.April, 17, 6, 0, 0, 0, time.LocationUTC),
	} {
		ctx := coord.NewContext(at, site, atmosphere.Refraction{})

		d, err := sat.GetDetails(ctx, DetailOverrides{})
		if err != nil {
			t.Fatalf("GetDetails: %v", err)
		}

		look, err := LookAngle(prov, 0, ctx)
		if err != nil {
			t.Fatalf("LookAngle: %v", err)
		}

		if d.DistanceUnit != "km" {
			t.Errorf("%v: distance unit %q, want km", at, d.DistanceUnit)
		}

		if got, want := d.Distance.Km(), look.Dist().Km(); want == 0 || math.Abs(got-want) > 0.001 {
			t.Errorf("%v: details distance %.3f km, LookAngle range %.3f km", at, got, want)
		}
	}
}
