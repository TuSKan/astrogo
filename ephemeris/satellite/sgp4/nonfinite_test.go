package sgp4_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
	"github.com/TuSKan/astrogo/vector"
)

// Two of Vallado's verification cases, one for each resonance the deep-space
// integrator steps through: MOLNIYA 2-14 (12 h) and ITALSAT 2 (24 h).
const (
	molniyaLine1 = "1 08195U 75081A   06176.33215444  .00000099  00000-0  11873-3 0   813"
	molniyaLine2 = "2 08195  64.1586 279.0717 6877146 264.7651  20.2257  2.00491383225656"
	italsatLine1 = "1 24208U 96044A   06177.04061740 -.00000094  00000-0  10000-3 0  1600"
	italsatLine2 = "2 24208   3.8536  80.0121 0026640 311.0977  48.3000  1.00778054 36119"
)

// withColumns writes text right-aligned into the 1-based, inclusive columns
// [from, to] of a TLE line, as the format writes a number.
func withColumns(line string, from, to int, text string) string {
	return line[:from-1] + fmt.Sprintf("%*s", to-from+1, text) + line[to:]
}

func mustPropagator(t *testing.T, line1, line2 string) *sgp4.Propagator {
	t.Helper()

	el, err := sgp4.ParseTLE(line1, line2)
	if err != nil {
		t.Fatalf("ParseTLE: %v", err)
	}

	p, err := sgp4.New(el)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return p
}

// TestAtRefusesANonFiniteTime is #1052's hang. The resonance integrator steps
// from epoch toward the requested time and stops only when it is within one
// step of it, which a NaN or an infinity never is: both resonant cases below
// ran forever. The near-Earth case returned a NaN state as a convergence
// failure instead.
func TestAtRefusesANonFiniteTime(t *testing.T) {
	t.Parallel()

	nearEarth, err := sgp4.New(leo())
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		p    *sgp4.Propagator
	}{
		{"near Earth", nearEarth},
		{"12 h resonance", mustPropagator(t, molniyaLine1, molniyaLine2)},
		{"24 h resonance", mustPropagator(t, italsatLine1, italsatLine2)},
	} {
		if !c.p.DeepSpace() && c.name != "near Earth" {
			t.Fatalf("%s is not a deep-space element set, so it does not reach the integrator", c.name)
		}

		for _, ts := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			pos, vel, err := c.p.At(ts)
			if !errors.Is(err, sgp4.ErrNotFinite) {
				t.Errorf("%s: At(%g) returned %v, want ErrNotFinite", c.name, ts, err)
			}

			if pos != (vector.Vec3{}) || vel != (vector.Vec3{}) {
				t.Errorf("%s: At(%g) returned a state (%v, %v) alongside its error", c.name, ts, pos, vel)
			}
		}
	}
}

// TestAtNeverReturnsANonFiniteState holds At to the promise its two advisory
// errors make: a state that comes back with an error exists. Before #1052 a
// finite but absurd input overflowed to a NaN state labeled
// ErrKeplerNotConverged, the error that means "use this with care".
func TestAtNeverReturnsANonFiniteState(t *testing.T) {
	t.Parallel()

	dragless := leo()
	dragless.BStar = 0

	absurd := leo()
	absurd.MeanMotion = 1e300

	for _, c := range []struct {
		name   string
		el     sgp4.Elements
		tsince float64
	}{
		// The drag-free time polynomials overflow past about 1e154 minutes.
		{"a time 1e200 minutes from epoch", dragless, 1e200},
		{"a time -1e200 minutes from epoch", dragless, -1e200},
		// A finite mean motion is a possible element set, as Validate defines
		// it, and this one overflows the model's initialization.
		{"a mean motion of 1e300 revolutions per day", absurd, 0},
	} {
		p, err := sgp4.New(c.el)
		if err != nil {
			t.Fatalf("%s: New: %v", c.name, err)
		}

		pos, vel, err := p.At(c.tsince)
		if !errors.Is(err, sgp4.ErrNotFinite) {
			t.Errorf("%s: At returned %v, want ErrNotFinite", c.name, err)
		}

		if pos != (vector.Vec3{}) || vel != (vector.Vec3{}) {
			t.Errorf("%s: At returned the state (%v, %v) alongside its error", c.name, pos, vel)
		}
	}
}

// TestValidateRefusesNonFiniteElements covers every numeric field. Before
// #1052 a NaN or infinite mean motion, RAAN, argument of perigee or mean
// anomaly passed Validate and New, and every propagation answered NaN.
func TestValidateRefusesNonFiniteElements(t *testing.T) {
	t.Parallel()

	fields := []struct {
		name string
		set  func(*sgp4.Elements, float64)
	}{
		{"MeanMotion", func(e *sgp4.Elements, v float64) { e.MeanMotion = v }},
		{"Eccentricity", func(e *sgp4.Elements, v float64) { e.Eccentricity = v }},
		{"Inclination", func(e *sgp4.Elements, v float64) { e.Inclination = angle.Angle(v) }},
		{"RAAN", func(e *sgp4.Elements, v float64) { e.RAAN = angle.Angle(v) }},
		{"ArgPerigee", func(e *sgp4.Elements, v float64) { e.ArgPerigee = angle.Angle(v) }},
		{"MeanAnomaly", func(e *sgp4.Elements, v float64) { e.MeanAnomaly = angle.Angle(v) }},
		{"BStar", func(e *sgp4.Elements, v float64) { e.BStar = v }},
		{"MeanMotionDot", func(e *sgp4.Elements, v float64) { e.MeanMotionDot = v }},
		{"MeanMotionDDot", func(e *sgp4.Elements, v float64) { e.MeanMotionDDot = v }},
	}

	for _, f := range fields {
		for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			el := leo()
			f.set(&el, v)

			if err := el.Validate(); !errors.Is(err, sgp4.ErrElements) {
				t.Errorf("%s = %g: Validate returned %v, want ErrElements", f.name, v, err)
			}

			if _, err := sgp4.New(el); !errors.Is(err, sgp4.ErrElements) {
				t.Errorf("%s = %g: New returned %v, want ErrElements", f.name, v, err)
			}
		}
	}
}

// TestParseTLERefusesTextThatIsNotADecimalNumber is the parser's half of
// #1052. strconv.ParseFloat reads NaN, Inf, infinity and hex floats, so a
// decimal column holding one of them parsed; for the epoch day a NaN then
// passed a range check that only NaN can pass.
func TestParseTLERefusesTextThatIsNotADecimalNumber(t *testing.T) {
	t.Parallel()

	columns := []struct {
		name     string
		line     int
		from, to int
	}{
		{"epoch day", 1, 21, 32},
		{"first derivative of mean motion", 1, 34, 43},
		{"inclination", 2, 9, 16},
		{"RAAN", 2, 18, 25},
		{"argument of perigee", 2, 35, 42},
		{"mean anomaly", 2, 44, 51},
		{"mean motion", 2, 53, 63},
	}

	for _, c := range columns {
		for _, text := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "infinity", "0x1p4", "1e2"} {
			l1, l2 := issLine1, issLine2
			if c.line == 1 {
				l1 = withColumns(l1, c.from, c.to, text)
			} else {
				l2 = withColumns(l2, c.from, c.to, text)
			}

			el, err := sgp4.ParseTLE(l1, l2)
			if !errors.Is(err, sgp4.ErrMalformedTLE) {
				t.Errorf("%s = %q: ParseTLE returned %v, want ErrMalformedTLE", c.name, text, err)
			}

			if el != (sgp4.Elements{}) {
				t.Errorf("%s = %q: ParseTLE returned elements alongside its error", c.name, text)
			}
		}
	}

	// The control: the same columns with the numbers they held still parse,
	// so the refusals above are about the text and not the splicing.
	for _, c := range columns {
		l1, l2 := issLine1, issLine2

		line := l2
		if c.line == 1 {
			line = l1
		}

		original := line[c.from-1 : c.to]
		if c.line == 1 {
			l1 = withColumns(l1, c.from, c.to, original)
		} else {
			l2 = withColumns(l2, c.from, c.to, original)
		}

		if _, err := sgp4.ParseTLE(l1, l2); err != nil {
			t.Errorf("%s rewritten with its own text %q no longer parses: %v", c.name, original, err)
		}
	}
}
