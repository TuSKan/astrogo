package coord

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// TestTopocentricToObservedIsSOFAsCIRSToObserved: a topocentric apparent
// direction carried to CIRS by the Context's own precession-nutation matrix
// and handed to SOFA's iauAtioq through CIRSToObserved lands where
// TopocentricToObserved puts it, with refraction off and on, over the whole
// sky above the horizon. The two share the Context's state and nothing of the
// rotation's arithmetic.
func TestTopocentricToObservedIsSOFAsCIRSToObserved(t *testing.T) {
	t.Parallel()

	site, err := NewEarthLocation(-24.6272, -70.4045, 2635)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, time.June, 21, 3, 0, 0, 0, time.LocationUTC)

	for _, atm := range []atmosphere.Refraction{{}, {Pressure: 744, Temperature: 10, Humidity: 0.2, Wavelength: 0.55}} {
		ctx := NewContext(at, site, atm)

		var worst float64

		for ra := 0.0; ra < 360; ra += 7.3 {
			for dec := -85.0; dec <= 85; dec += 4.9 {
				v := vector.FromSpherical(ra*math.Pi/180, dec*math.Pi/180).MulScalar(2.7)

				got := ctx.TopocentricToObserved(v)
				if got.Alt().Degrees() < 12 {
					continue
				}

				m := ctx.rc2i
				cx := m[0][0]*v.X + m[0][1]*v.Y + m[0][2]*v.Z
				cy := m[1][0]*v.X + m[1][1]*v.Y + m[1][2]*v.Z
				cz := m[2][0]*v.X + m[2][1]*v.Y + m[2][2]*v.Z
				cra, cdec := vector.V3(cx, cy, cz).ToSpherical()

				want := ctx.CIRSToObserved(NewCIRS(angle.Rad(cra), angle.Rad(cdec)))

				worst = math.Max(worst, separation(got, want))

				if d := got.Dist().AU(); math.Abs(d-2.7) > 1e-15 {
					t.Fatalf("distance %v, want the input's 2.7 au", d)
				}
			}
		}

		t.Logf("pressure %v hPa: worst %.3g µas from Atioq", atm.Pressure, worst*1e6)

		// Measured 0.00027 µas: the two rotations' rounding, at the scale of
		// a double's last bit on a unit vector. 0.01 µas leaves arm64's fused
		// multiply-adds room and catches any term either rotation drops.
		if worst > 1e-8 {
			t.Errorf("pressure %v hPa: TopocentricToObserved is %.3g µas from SOFA's Atioq", atm.Pressure, worst*1e6)
		}
	}
}

// separation is the angle between two observed places, in arcseconds.
func separation(a, b AltAz) float64 {
	u := vector.FromSpherical(a.Az().Radians(), a.Alt().Radians())
	w := vector.FromSpherical(b.Az().Radians(), b.Alt().Radians())

	return math.Atan2(u.Cross(w).Norm(), u.Dot(w)) * 180 / math.Pi * 3600
}
