package angle_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// Wrapping must land inside the range each function documents, for every input
// including the ones that arise from subtracting two nearly equal angles.
//
// This is what found Wrap2Pi returning exactly 2*pi. The range is half-open, so
// 2*pi is the single value it must never produce, and adding 2*pi to any
// negative smaller than half an ULP of it has no representable sum other than
// 2*pi itself. Deg(-1e-14).Wrap360() came out as 360 degrees.
func TestWrapRangesAreRespected(t *testing.T) {
	t.Parallel()

	const twoPi = 2 * math.Pi

	// math.Copysign, not the literal -0.0: Go parses that as positive zero,
	// so writing it would have quietly tested the same case twice.
	negativeZero := math.Copysign(0, -1)

	inputs := []float64{
		0, negativeZero,
		math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		1e-300, -1e-300, 1e-18, -1e-18, 1e-16, -1e-16,
		-4.4e-16, -4.5e-16, // either side of half an ULP of 2*pi
		math.Nextafter(0, -1), math.Nextafter(twoPi, 0), math.Nextafter(twoPi, 10),
		math.Pi, -math.Pi, twoPi, -twoPi, 3 * math.Pi, -3 * math.Pi,
		1, -1, 100, -100, 1e6, -1e6,
	}

	for _, v := range inputs {
		if got := angle.Rad(v).Wrap2Pi().Radians(); !(got >= 0 && got < twoPi) {
			t.Errorf("Wrap2Pi(%g) = %.20g, outside the documented [0, 2pi)", v, got)
		}

		if got := angle.Rad(v).WrapPi().Radians(); !(got > -math.Pi && got <= math.Pi) {
			t.Errorf("WrapPi(%g) = %.20g, outside the documented (-pi, pi]", v, got)
		}
	}

	// Degrees, because that is the form a caller reads and the form the bug
	// showed itself in.
	for _, d := range []float64{-1e-14, -1e-12, -1e-9, -0.5, 0, 0.5, 359.5, 360, 360.5, 720, -720} {
		if got := angle.Deg(d).Wrap360().Degrees(); !(got >= 0 && got < 360) {
			t.Errorf("Deg(%g).Wrap360() = %.17g degrees, outside [0, 360)", d, got)
		}
	}

	// Wrapping must not move an angle that is already in range, beyond the
	// rounding of the modulo itself.
	for _, v := range []float64{0, 0.1, 1, 3, 6, 6.28} {
		if got := angle.Rad(v).Wrap2Pi().Radians(); math.Abs(got-v) > 1e-15 {
			t.Errorf("Wrap2Pi(%g) moved an in-range angle to %g", v, got)
		}
	}

	// Wrapping is idempotent: the second application changes nothing.
	for _, v := range inputs {
		once := angle.Rad(v).Wrap2Pi()
		if twice := once.Wrap2Pi(); twice != once {
			t.Errorf("Wrap2Pi(%g) = %v but wrapping again gave %v", v, once.Radians(), twice.Radians())
		}

		oncePi := angle.Rad(v).WrapPi()
		if twicePi := oncePi.WrapPi(); twicePi != oncePi {
			t.Errorf("WrapPi(%g) = %v but wrapping again gave %v", v, oncePi.Radians(), twicePi.Radians())
		}
	}

	// A wrapped angle names the same direction as the one it came from.
	for _, v := range []float64{-3 * math.Pi, -100, -1, 0.5, 7, 100, 1e6} {
		w := angle.Rad(v).Wrap2Pi().Radians()
		if math.Abs(math.Sin(w)-math.Sin(v)) > 1e-9 || math.Abs(math.Cos(w)-math.Cos(v)) > 1e-9 {
			t.Errorf("Wrap2Pi(%g) = %g names a different direction", v, w)
		}
	}
}

// The formatted forms must stay well formed for every input, and must keep the
// sign — a declination that renders without its minus is a target on the wrong
// side of the equator.
func TestSexagesimalFormattingIsWellFormed(t *testing.T) {
	t.Parallel()

	values := []float64{
		0, math.Copysign(0, -1), 1e-9, -1e-9, 0.5 / 3600, -0.5 / 3600,
		1.0 / 3600, -1.0 / 3600, 59.999 / 3600, -59.999 / 3600,
		1, -1, 9.9999999, -9.9999999, 59.9999999 / 60, 89.9999999, -89.9999999,
		90, -90, 179.9999999, 180, -180, 359.9999999, 360,
	}

	for _, precision := range []int{-1, 0, 1, 2, 3, 6} {
		for _, d := range values {
			a := angle.Deg(d)

			dms := a.DMSString(precision)
			if err := wellFormedDMS(dms); err != "" {
				t.Errorf("DMSString(%g, precision %d) = %q: %s", d, precision, dms, err)
			}

			// The sign must survive, including for a value that rounds to zero.
			if math.Signbit(d) && d != 0 && !strings.HasPrefix(dms, "-") {
				t.Errorf("DMSString(%g, precision %d) = %q lost the minus sign", d, precision, dms)
			}

			hms := a.HMSString(precision)
			if err := wellFormedHMS(hms); err != "" {
				t.Errorf("HMSString(%g, precision %d) = %q: %s", d, precision, hms, err)
			}
		}
	}
}

// wellFormedDMS returns a reason the string is malformed, or "" if it is fine.
func wellFormedDMS(s string) string {
	if len(s) < 2 || (s[0] != '+' && s[0] != '-') {
		return "no leading sign"
	}

	body := s[1:]

	deg, rest, ok := strings.Cut(body, "\u00b0")
	if !ok {
		return "no degree mark"
	}

	minutes, seconds, ok := strings.Cut(rest, "'")
	if !ok {
		return "no minute mark"
	}

	if !strings.HasSuffix(seconds, "\"") {
		return "no second mark"
	}

	seconds = strings.TrimSuffix(seconds, "\"")

	if len(deg) < 2 {
		return "degrees not zero padded"
	}

	if len(minutes) != 2 {
		return "minutes not two digits"
	}

	// The carry rules: 60 minutes or 60 seconds must have been carried, never
	// rendered.
	if minutes >= "60" {
		return "minutes reached 60 without carrying"
	}

	whole, _, _ := strings.Cut(seconds, ".")
	if len(whole) != 2 {
		return "seconds not two whole digits"
	}

	if whole >= "60" {
		return "seconds reached 60 without carrying"
	}

	return ""
}

// wellFormedHMS is the same check for the hour form, which has no sign because
// it is wrapped into [0h, 24h).
func wellFormedHMS(s string) string {
	hours, rest, ok := strings.Cut(s, "h")
	if !ok {
		return "no hour mark"
	}

	minutes, seconds, ok := strings.Cut(rest, "m")
	if !ok {
		return "no minute mark"
	}

	if !strings.HasSuffix(seconds, "s") {
		return "no second mark"
	}

	seconds = strings.TrimSuffix(seconds, "s")

	if len(hours) != 2 || hours >= "24" {
		return "hours not two digits below 24"
	}

	if len(minutes) != 2 || minutes >= "60" {
		return "minutes reached 60 without carrying"
	}

	whole, _, _ := strings.Cut(seconds, ".")
	if len(whole) != 2 || whole >= "60" {
		return "seconds reached 60 without carrying"
	}

	return ""
}

// Every unit accessor must invert its constructor exactly.
func TestUnitAccessorsInvertTheirConstructors(t *testing.T) {
	t.Parallel()

	for _, v := range []float64{0, 1e-12, 0.5, 1, 23.9, 180, 360, -47.3, 1e6} {
		pairs := []struct {
			name string
			out  float64
			want float64
		}{
			{"Deg/Degrees", angle.Deg(v).Degrees(), v},
			{"Rad/Radians", angle.Rad(v).Radians(), v},
			{"Arcmin/Arcminutes", angle.Arcmin(v).Arcminutes(), v},
			{"Arcsec/Arcseconds", angle.Arcsec(v).Arcseconds(), v},
			{"Hour/Hours", angle.Hour(v).Hours(), v},
		}

		for _, p := range pairs {
			if p.want == 0 {
				if p.out != 0 {
					t.Errorf("%s(%g) = %g, want 0", p.name, v, p.out)
				}

				continue
			}

			if rel := math.Abs(p.out-p.want) / math.Abs(p.want); rel > 1e-15 {
				t.Errorf("%s(%g) = %.17g, a relative error of %.3g", p.name, v, p.out, rel)
			}
		}
	}

	// And the units must agree with each other.
	a := angle.Deg(1)
	if got := a.Arcminutes(); math.Abs(got-60) > 1e-12 {
		t.Errorf("one degree is %g arcminutes, want 60", got)
	}

	if got := a.Arcseconds(); math.Abs(got-3600) > 1e-9 {
		t.Errorf("one degree is %g arcseconds, want 3600", got)
	}

	if got := angle.Hour(1).Degrees(); math.Abs(got-15) > 1e-12 {
		t.Errorf("one hour of right ascension is %g degrees, want 15", got)
	}
}
