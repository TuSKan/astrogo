package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestMoonIlluminationMatchesSkyfield checks the phase angle and illuminated
// fraction against Skyfield 1.54's almanac.phase_angle and
// fraction_illuminated (DE421) at the three instants of #416. Before the fix
// the "phase angle" was the elongation, 110.787° where the phase angle is
// 69.084°, and the fraction was off by up to 0.0014. Meeus's Example 48.a,
// the first instant, gives 69.0756° and 0.6786 from his truncated lunar
// theory.
func TestMoonIlluminationMatchesSkyfield(t *testing.T) {
	for _, c := range []struct {
		at        time.Time
		i, k, psi float64
	}{
		// 1992-04-12 0h TD.
		{time.Date(1992, 4, 11, 23, 59, 1, 700000000, time.LocationUTC), 69.084, 0.67850, 110.792},
		{time.Date(2026, 1, 26, 4, 48, 0, 0, time.LocationUTC), 89.856, 0.50126, 90.006},
		{time.Date(2026, 9, 18, 7, 0, 0, 0, time.LocationUTC), 96.068, 0.44714, 83.784},
	} {
		k, i, err := MoonIllumination(c.at, eph.Default())
		if err != nil {
			t.Fatal(err)
		}

		if math.Abs(i.Degrees()-c.i) > 0.003 {
			t.Errorf("%v: phase angle %.4f°, Skyfield gives %.3f° (the elongation is %.3f°)",
				c.at, i.Degrees(), c.i, c.psi)
		}

		if math.Abs(k-c.k) > 0.00003 {
			t.Errorf("%v: illuminated fraction %.5f, Skyfield gives %.5f", c.at, k, c.k)
		}
	}
}

// TestMoonPhaseAngleIsNotTheElongation: the phase angle i, the elongation ψ
// and the angle at the Sun close the Sun–Earth–Moon triangle, so 180° − ψ − i
// is the Earth–Sun–Moon angle, never negative and never above 0.15°. Returning
// ψ as i, as before #416, puts it anywhere from −180° to 180°.
func TestMoonPhaseAngleIsNotTheElongation(t *testing.T) {
	prov := eph.Default()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.LocationUTC)

	largest := 0.0

	for h := 0.0; h < 30*24; h += 7 {
		at := start.Add(unit.Hours(h))

		_, i, err := MoonIllumination(at, prov)
		if err != nil {
			t.Fatal(err)
		}

		psi := geocentricSeparation(t, prov, eph.Moon, eph.Sun, at)

		// The elongation here is to the geometric Moon, the phase angle's to the
		// astrometric one: they disagree by up to 0.006°.
		atSun := 180 - psi - i.Degrees()
		if atSun < -0.01 || atSun > 0.16 {
			t.Fatalf("%v: phase angle %.4f° and elongation %.4f° leave %.4f° at the Sun, want [0°, 0.15°]",
				at, i.Degrees(), psi, atSun)
		}

		largest = max(largest, atSun)
	}

	if largest < 0.12 {
		t.Errorf("the angle at the Sun peaked at %.4f° over a lunation, want near 0.15° at the quarters", largest)
	}
}

// TestQuarterMoonsAreNotExactlyHalfLit: an EventSolver lunar phase carries the
// Moon's illuminated fraction as its Value. At a quarter the elongation is 90°
// but the phase angle is 0.15° less, so the Moon is 50.13% lit. Before #416
// Value was (1 − cos Δλ)/2 of the very longitude difference the solver had just
// set to 90°, and so exactly 0.5.
func TestQuarterMoonsAreNotExactlyHalfLit(t *testing.T) {
	prov := eph.Default()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, kind := range []EventKind{EventFirstQuarter, EventLastQuarter} {
		events, err := NewEventSolver(unit.Hours(6), unit.Seconds(1)).Find(EventSpec{
			Family: EventFamilyIllumination, Kind: kind, Target: NewMoon(prov),
		}, start, start.Add(unit.Days(365)))
		if err != nil {
			t.Fatal(err)
		}

		if len(events) < 12 {
			t.Fatalf("%v: %d events in 2026, want 12 or 13", kind, len(events))
		}

		for _, e := range events {
			k, _, err := MoonIllumination(e.Time, prov)
			if err != nil {
				t.Fatal(err)
			}

			if e.Value != k || k < 0.5010 || k > 0.5016 {
				t.Errorf("%v at %v: Value %.5f, MoonIllumination %.5f, want both near 0.5013", kind, e.Time, e.Value, k)
			}
		}
	}
}

// geocentricSeparation is the angle between two bodies' geometric geocentric
// directions, in degrees.
func geocentricSeparation(t *testing.T, prov eph.Provider, a, b eph.ID, at time.Time) float64 {
	t.Helper()

	var dirs [2]coord.ICRS

	for n, id := range []eph.ID{a, b} {
		pos, err := eph.Position(prov, id, at)
		if err != nil {
			t.Fatal(err)
		}

		if dirs[n], err = eph.ToICRS(pos); err != nil {
			t.Fatal(err)
		}
	}

	return coord.Separation(dirs[0], dirs[1]).Degrees()
}
