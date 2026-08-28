package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// The depth is the integral of the extinction, checked numerically.
//
// # Why this pairing is the thing to test
//
// The two functions describe one profile and are used together: transmission
// comes from the depth and the scattering source term from the extinction. If
// they disagree, the atmosphere attenuates at one rate and scatters at
// another, and the result is smooth, positive and wrong everywhere. Neither
// function alone can catch that; only the relationship between them can.
func TestExponentialDepthIntegratesItsExtinction(t *testing.T) {
	t.Parallel()

	const (
		column      = unit.OpticalDepth(0.35)
		scaleHeight = 8000.0
		top         = 40000.0
		steps       = 200000
	)

	// Trapezoidal, fine enough that the quadrature error is far below the
	// tolerance and any failure is the formulae disagreeing.
	var sum float64

	step := top / steps

	for i := range steps {
		lo, err := atmosphere.ExponentialExtinction(unit.AltitudeM(float64(i)*step), column, scaleHeight)
		if err != nil {
			t.Fatalf("ExponentialExtinction: %v", err)
		}

		hi, err := atmosphere.ExponentialExtinction(unit.AltitudeM(float64(i+1)*step), column, scaleHeight)
		if err != nil {
			t.Fatalf("ExponentialExtinction: %v", err)
		}

		sum += 0.5 * (lo + hi) * step
	}

	got, err := atmosphere.ExponentialDepth(top, column, scaleHeight)
	if err != nil {
		t.Fatalf("ExponentialDepth: %v", err)
	}

	if rel := math.Abs(sum-float64(got)) / float64(got); rel > 1e-6 {
		t.Errorf("the integral of the extinction is %.9g and the depth is %.9g, a relative "+
			"difference of %.3g; they describe the same profile and must agree", sum, float64(got), rel)
	}
}

// The whole column is recovered at the top of the atmosphere.
//
// This is what makes the two arguments mean what they claim: a caller who
// passes a column of 0.35 has specified an atmosphere with exactly that much
// extinction in it, not one that happens to approach it.
func TestExponentialDepthTendsToTheColumn(t *testing.T) {
	t.Parallel()

	const (
		column      = unit.OpticalDepth(0.35)
		scaleHeight = 8000.0
	)

	// Twenty scale heights: exp(-20) is 2e-9, so this is the column to well
	// inside any tolerance worth asserting.
	got, err := atmosphere.ExponentialDepth(20*scaleHeight, column, scaleHeight)
	if err != nil {
		t.Fatalf("ExponentialDepth: %v", err)
	}

	if rel := math.Abs(float64(got-column)) / float64(column); rel > 1e-8 {
		t.Errorf("at twenty scale heights the depth is %.9g against a column of %.9g", got, column)
	}

	// And nothing at the ground, since there is no atmosphere below it.
	zero, err := atmosphere.ExponentialDepth(0, column, scaleHeight)
	if err != nil {
		t.Fatalf("ExponentialDepth: %v", err)
	}

	if zero != 0 {
		t.Errorf("the depth at the ground is %g, want 0", float64(zero))
	}
}

// The extinction falls by e over one scale height, which is what a scale
// height is.
func TestExponentialExtinctionDecaysByE(t *testing.T) {
	t.Parallel()

	const (
		column      = unit.OpticalDepth(0.35)
		scaleHeight = 8000.0
	)

	ground, err := atmosphere.ExponentialExtinction(0, column, scaleHeight)
	if err != nil {
		t.Fatalf("ExponentialExtinction: %v", err)
	}

	// At the ground it is tau_0 / H exactly.
	if want := float64(column) / scaleHeight; math.Abs(ground-want)/want > 1e-12 {
		t.Errorf("ground extinction %.9g, want %.9g", ground, want)
	}

	up, err := atmosphere.ExponentialExtinction(unit.AltitudeM(scaleHeight), column, scaleHeight)
	if err != nil {
		t.Fatalf("ExponentialExtinction: %v", err)
	}

	if ratio := ground / up; math.Abs(ratio-math.E)/math.E > 1e-12 {
		t.Errorf("one scale height up the extinction falls by %.9g, want e", ratio)
	}
}

// Both functions reject what they cannot describe.
func TestProfileRejectsBadArguments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		alt         unit.AltitudeM
		column      unit.OpticalDepth
		scaleHeight float64
		want        error
	}{
		{"zero scale height", 1000, 0.3, 0, atmosphere.ErrScaleHeight},
		{"negative scale height", 1000, 0.3, -8000, atmosphere.ErrScaleHeight},
		{"NaN scale height", 1000, 0.3, math.NaN(), atmosphere.ErrScaleHeight},
		{"negative column", 1000, -0.3, 8000, atmosphere.ErrColumnDepth},
		{"infinite column", 1000, unit.OpticalDepth(math.Inf(1)), 8000, atmosphere.ErrColumnDepth},
		{"negative altitude", -10, 0.3, 8000, atmosphere.ErrAltitude},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if _, err := atmosphere.ExponentialExtinction(c.alt, c.column, c.scaleHeight); !errors.Is(err, c.want) {
				t.Errorf("ExponentialExtinction: got %v, want %v", err, c.want)
			}

			if _, err := atmosphere.ExponentialDepth(c.alt, c.column, c.scaleHeight); !errors.Is(err, c.want) {
				t.Errorf("ExponentialDepth: got %v, want %v", err, c.want)
			}
		})
	}
}

// The volume scattering function integrates to the total scattering
// coefficient over the sphere.
//
// # Why this is the test that matters
//
// Kocifaj (2007) Eq. 18 carries an explicit 1/4*pi and this package's phase
// functions carry their own normalisation, so transcribing the equation
// literally divides the whole model by 4*pi. That error is a constant factor:
// every radiance stays positive, every angular shape stays right, and nothing
// downstream looks wrong. Integrating over the sphere is what distinguishes
// them, because it has a value the physics fixes — the total scattering
// coefficient, no more and no less.
func TestVolumeScatteringFunctionIntegratesToTheScatteringCoefficient(t *testing.T) {
	t.Parallel()

	const (
		molecular = 1.2e-5 // m^-1
		aerosol   = 8.0e-6
		g         = 0.65
		depol     = 0.0279
		steps     = 200000
	)

	// Integral over the sphere of beta(theta) dOmega, with dOmega =
	// 2*pi*sin(theta)*dtheta by azimuthal symmetry.
	var sum float64

	step := math.Pi / steps

	for i := range steps {
		mid := (float64(i) + 0.5) * step

		beta, err := atmosphere.VolumeScatteringFunction(mid, molecular, aerosol, g, depol)
		if err != nil {
			t.Fatalf("VolumeScatteringFunction: %v", err)
		}

		sum += beta * 2 * math.Pi * math.Sin(mid) * step
	}

	want := molecular + aerosol
	if rel := math.Abs(sum-want) / want; rel > 1e-6 {
		t.Errorf("the volume scattering function integrates to %.9g over the sphere and the "+
			"total scattering coefficient is %.9g, a relative difference of %.3g — a factor of "+
			"4*pi here would read as %.4g", sum, want, rel, sum/want)
	}
}

// Aerosol forward-scattering shows up where it should.
//
// A positive asymmetry parameter puts more light forward than back, and the
// molecular term is symmetric about 90 degrees, so the whole function has to
// be forward-weighted whenever there is any aerosol at all.
func TestVolumeScatteringFunctionIsForwardWeighted(t *testing.T) {
	t.Parallel()

	const (
		molecular = 1.2e-5
		aerosol   = 8.0e-6
		g         = 0.65
		depol     = 0.0279
	)

	fwd, err := atmosphere.VolumeScatteringFunction(0.1, molecular, aerosol, g, depol)
	if err != nil {
		t.Fatalf("VolumeScatteringFunction: %v", err)
	}

	back, err := atmosphere.VolumeScatteringFunction(math.Pi-0.1, molecular, aerosol, g, depol)
	if err != nil {
		t.Fatalf("VolumeScatteringFunction: %v", err)
	}

	if fwd <= back {
		t.Errorf("forward %.4g is not above backward %.4g at g = %g", fwd, back, g)
	}

	// With no aerosol the Rayleigh term alone is symmetric about 90 degrees.
	a, err := atmosphere.VolumeScatteringFunction(0.4, molecular, 0, g, depol)
	if err != nil {
		t.Fatalf("VolumeScatteringFunction: %v", err)
	}

	b, err := atmosphere.VolumeScatteringFunction(math.Pi-0.4, molecular, 0, g, depol)
	if err != nil {
		t.Fatalf("VolumeScatteringFunction: %v", err)
	}

	if rel := math.Abs(a-b) / a; rel > 1e-12 {
		t.Errorf("the molecular term is asymmetric by %.3g; Rayleigh is symmetric about 90 degrees", rel)
	}
}
