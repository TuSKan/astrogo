package gofaext_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestEcm06IsEqec06sFrame holds the matrix to the conversion that applies it:
// a direction rotated by Ecm06 has the ecliptic longitude and latitude Eqec06
// gives, over a sky of directions and dates from 1900 to 2100. The matrix
// must be the one from ICRS, not its transpose: a transposed one moves every
// direction by twice the precession and obliquity, degrees, not rounding.
func TestEcm06IsEqec06sFrame(t *testing.T) {
	t.Parallel()

	const tol = 1e-14 // radians: rounding, 2e-9″

	for tt1 := 2415020.5; tt1 < 2488069.5; tt1 += 3652.5 {
		rm := gofaext.Ecm06(tt1, 0.25)

		for ra := 0.1; ra < 2*math.Pi; ra += 0.7 {
			for dec := -1.4; dec < 1.5; dec += 0.35 {
				v := [3]float64{math.Cos(dec) * math.Cos(ra), math.Cos(dec) * math.Sin(ra), math.Sin(dec)}

				var e [3]float64
				for i := range 3 {
					e[i] = rm[i][0]*v[0] + rm[i][1]*v[1] + rm[i][2]*v[2]
				}

				lon, lat := gofaext.Eqec06(tt1, 0.25, ra, dec)
				gotLon := math.Atan2(e[1], e[0])
				gotLat := math.Atan2(e[2], math.Hypot(e[0], e[1]))

				if d := math.Abs(math.Remainder(gotLon-lon, 2*math.Pi)); d > tol || math.Abs(gotLat-lat) > tol {
					t.Fatalf("TT %.1f, RA %.2f, Dec %.2f: Ecm06 gives (%.15f, %.15f), Eqec06 (%.15f, %.15f)",
						tt1, ra, dec, gotLon, gotLat, lon, lat)
				}
			}
		}
	}
}
