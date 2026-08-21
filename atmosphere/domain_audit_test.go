package atmosphere_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// finite reports whether a value is usable as a physical quantity.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Airmass must answer or refuse, never return a number that is not one.
//
// The horizon is where every airmass formula misbehaves: the plane-parallel
// secant diverges there, and the empirical fits are only defined above some
// altitude. Below the horizon there is no path through the atmosphere at all.
//
// Zero altitude is not below the horizon, it is on it — a real if extreme
// sightline, and a finite airmass of a few tens is the right answer rather than
// an error. Only a negative altitude is off the sky.
func TestAirmassDomain(t *testing.T) {
	t.Parallel()

	// Both are empirical fits, not derivations, so neither lands on exactly one
	// at the zenith and the tolerance is each fit's own published residual.
	//
	// Pickering's is the more interesting of the two: its inner argument
	// h + 244/(165 + 47*h^1.1) crosses 90 degrees before the altitude does, so
	// the minimum is at 89.964 degrees and the value rises by 1.96e-7 over the
	// last 0.036 degrees. That is a property of the fit, which is why the
	// monotonic check carries a tolerance rather than demanding a strict fall.
	for _, fn := range []struct {
		name            string
		f               func(angle.Angle) (float64, error)
		zenithTolerance float64
		monotoneSlack   float64
	}{
		{"Airmass", atmosphere.Airmass, 1e-6, 1e-6},
		{"GushchinAirmass", atmosphere.GushchinAirmass, 1e-4, 1e-9},
	} {
		for _, alt := range []float64{-90, -45, -1, -0.001} {
			if got, err := fn.f(angle.Deg(alt)); err == nil {
				t.Errorf("%s(%.3f) returned %v for a direction below the horizon", fn.name, alt, got)
			}
		}

		// Ascending altitudes, so the airmass must fall monotonically: a
		// shorter slant path through the same atmosphere.
		previous := math.Inf(1)

		for _, alt := range []float64{0, 0.001, 0.1, 1, 5, 15, 30, 45, 60, 80, 89, 89.999, 90} {
			got, err := fn.f(angle.Deg(alt))
			if err != nil {
				continue
			}

			if !finite(got) {
				t.Errorf("%s(%.3f) returned %v with no error", fn.name, alt, got)

				continue
			}

			if got < 1 {
				t.Errorf("%s(%.3f) = %.6f; the shortest path through the atmosphere is one airmass",
					fn.name, alt, got)
			}

			if got > previous*(1+fn.monotoneSlack) {
				t.Errorf("%s: airmass rose from %.9f to %.9f as the altitude climbed to %.3f degrees",
					fn.name, previous, got, alt)
			}

			previous = got
		}

		// At the zenith the airmass is one, by definition.
		if got, err := fn.f(angle.Deg(90)); err == nil && math.Abs(got-1) > fn.zenithTolerance {
			t.Errorf("%s at the zenith = %.9f, want 1 within %g", fn.name, got, fn.zenithTolerance)
		}

		// On the horizon it is large but finite. A plane-parallel secant would
		// diverge here; a real atmosphere gives a few tens.
		got, err := fn.f(0)
		if err == nil && (got < 10 || got > 100) {
			t.Errorf("%s on the horizon = %.3f, want a few tens", fn.name, got)
		}
	}
}

// Optical depth must be positive, finite, and fall with wavelength.
//
// Rayleigh scattering goes as lambda^-4, so the blue end is optically thicker
// than the red by a large factor. A sign or an inverted exponent leaves a
// spectrum that is smooth and positive and reddens the sky the wrong way.
func TestRayleighOpticalDepthDomain(t *testing.T) {
	t.Parallel()

	const seaLevel = 1013.25

	previous := math.Inf(1)

	for _, nm := range []float64{300, 350, 400, 500, 550, 600, 700, 800, 1000} {
		tau, err := atmosphere.RayleighOpticalDepth(unit.WavelengthNM(nm), seaLevel)
		if err != nil {
			t.Fatalf("RayleighOpticalDepth(%v): %v", nm, err)
		}

		v := float64(tau)
		if !finite(v) || v <= 0 {
			t.Fatalf("tau at %.0f nm = %v", nm, v)
		}

		if v >= previous {
			t.Errorf("tau rose from %.6f to %.6f going from blue to %.0f nm; Rayleigh falls as lambda^-4",
				previous, v, nm)
		}

		previous = v
	}

	// The textbook anchor: about 0.0973 at 550 nm at sea level.
	tau, err := atmosphere.RayleighOpticalDepth(550, seaLevel)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	if v := float64(tau); math.Abs(v-0.0973) > 0.005 {
		t.Errorf("Rayleigh optical depth at 550 nm, sea level = %.5f, want about 0.0973", v)
	}

	// Halving the pressure halves the column, and so the depth.
	half, err := atmosphere.RayleighOpticalDepth(550, seaLevel/2)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	if ratio := float64(half) / float64(tau); math.Abs(ratio-0.5) > 1e-9 {
		t.Errorf("half the pressure gave %.6f of the optical depth, want exactly half", ratio)
	}

	// Nonsense pressure must be refused rather than producing a negative depth.
	for _, p := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if got, err := atmosphere.RayleighOpticalDepth(550, p); err == nil && (!finite(float64(got)) || got < 0) {
			t.Errorf("pressure %v was accepted and gave tau %v", p, got)
		}
	}
}

// Transmission is a fraction: it never leaves [0, 1] and never rises with depth.
func TestTransmissionIsBounded(t *testing.T) {
	t.Parallel()

	previous := math.Inf(1)

	for _, tau := range []float64{0, 1e-12, 0.01, 0.1, 1, 5, 20, 100, 1000} {
		got := float64(atmosphere.Transmission(unit.OpticalDepth(tau)))

		if !finite(got) || got < 0 || got > 1 {
			t.Errorf("Transmission(%v) = %v, which is not a fraction", tau, got)

			continue
		}

		if got > previous {
			t.Errorf("Transmission rose from %.6g to %.6g as the depth increased to %v",
				previous, got, tau)
		}

		previous = got
	}

	if got := float64(atmosphere.Transmission(0)); math.Abs(got-1) > 1e-15 {
		t.Errorf("Transmission(0) = %v, want exactly 1", got)
	}
}

// A phase function must be positive everywhere and integrate to one over the
// sphere, which is what makes it a redistribution of light rather than a source
// or a sink of it.
func TestPhaseFunctionsAreNormalised(t *testing.T) {
	t.Parallel()

	// Integrate p(theta) over the sphere: 2*pi*Integral p(theta) sin(theta) dtheta.
	integrate := func(p func(float64) (float64, error)) (float64, error) {
		const steps = 200000

		var sum float64

		for i := range steps {
			theta := (float64(i) + 0.5) * math.Pi / steps

			v, err := p(theta)
			if err != nil {
				return 0, err
			}

			if !finite(v) || v < 0 {
				return 0, nil
			}

			sum += v * math.Sin(theta) * (math.Pi / steps)
		}

		return 2 * math.Pi * sum, nil
	}

	rayleigh := func(theta float64) (float64, error) {
		return atmosphere.RayleighPhaseFunction(theta, 0), nil
	}

	total, err := integrate(rayleigh)
	if err != nil {
		t.Fatalf("Rayleigh: %v", err)
	}

	if math.Abs(total-1) > 1e-3 {
		t.Errorf("the Rayleigh phase function integrates to %.6f over the sphere, want 1", total)
	}

	for _, g := range []float64{-0.8, -0.3, 0, 0.3, 0.65, 0.9} {
		hg := func(theta float64) (float64, error) {
			return atmosphere.HenyeyGreensteinPhaseFunction(theta, g)
		}

		total, err := integrate(hg)
		if err != nil {
			t.Fatalf("Henyey-Greenstein g=%v: %v", g, err)
		}

		if math.Abs(total-1) > 5e-3 {
			t.Errorf("Henyey-Greenstein with g=%v integrates to %.6f, want 1", g, total)
		}
	}
}

// van Rhijn is the airglow geometry: one at the zenith, growing toward the
// horizon, and refusing a line of sight that never reaches the layer.
func TestVanRhijnDomain(t *testing.T) {
	t.Parallel()

	const layer = 87_000

	if got, err := atmosphere.VanRhijn(0, layer); err != nil || math.Abs(got-1) > 1e-12 {
		t.Errorf("at the zenith VanRhijn = %v (err %v), want exactly 1", got, err)
	}

	previous := 0.0

	for _, z := range []float64{0, 10, 30, 50, 70, 85, 89, 89.99} {
		got, err := atmosphere.VanRhijn(angle.Deg(z), layer)
		if err != nil {
			continue
		}

		if !finite(got) || got < 1 {
			t.Errorf("VanRhijn(%.2f) = %v; the slant path is never shorter than the vertical one", z, got)

			continue
		}

		if got < previous {
			t.Errorf("VanRhijn fell from %.6f to %.6f on the way to %.2f degrees", previous, got, z)
		}

		previous = got
	}

	// A layer height that is not a height must be refused, not turned into a
	// silent enhancement.
	for _, h := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := atmosphere.VanRhijn(angle.Deg(30), h); err == nil {
			t.Errorf("layer height %v was accepted", h)
		}
	}
}
