package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
)

// Winkler (2022) Eq. 13 at 550 nm and sea level must reproduce the
// standard sea-level Rayleigh optical depth of ~0.0973-0.098, which is an
// independently known value rather than one taken from the same paper.
// This is the Level-1 equation check for the molecular optical depth.
func TestRayleighOpticalDepthSeaLevel550(t *testing.T) {
	t.Parallel()

	tau, err := atmosphere.RayleighOpticalDepth(550, atmosphere.StandardPressureHPa)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	if got := float64(tau); got < 0.096 || got > 0.100 {
		t.Errorf("tau_R(550 nm, sea level) = %.5f, want ~0.098", got)
	}
}

// The optical depth scales linearly with pressure, which is what makes a
// high observatory's molecular atmosphere thinner. Paranal at ~2635 m sits
// near 743 hPa, so it should see roughly 73 per cent of the sea-level
// value.
func TestRayleighOpticalDepthScalesWithPressure(t *testing.T) {
	t.Parallel()

	sea, err := atmosphere.RayleighOpticalDepth(550, atmosphere.StandardPressureHPa)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	const paranalHPa = 743.0

	high, err := atmosphere.RayleighOpticalDepth(550, paranalHPa)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	want := paranalHPa / atmosphere.StandardPressureHPa
	if got := float64(high / sea); math.Abs(got-want) > 1e-12 {
		t.Errorf("pressure scaling = %v, want %v", got, want)
	}
}

// Blue light scatters far more than red: the lambda^-4.05 dependence makes
// 400 nm roughly 5.5x deeper than 700 nm. A sign error or a lost exponent
// shows up immediately here.
func TestRayleighOpticalDepthSpectralSlope(t *testing.T) {
	t.Parallel()

	blue, err := atmosphere.RayleighOpticalDepth(400, atmosphere.StandardPressureHPa)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	red, err := atmosphere.RayleighOpticalDepth(700, atmosphere.StandardPressureHPa)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	if blue <= red {
		t.Fatalf("blue tau %v must exceed red tau %v", blue, red)
	}

	want := math.Pow(400.0/700.0, -4.05)
	if got := float64(blue / red); math.Abs(got-want)/want > 1e-12 {
		t.Errorf("tau(400)/tau(700) = %v, want %v", got, want)
	}
}

func TestRayleighOpticalDepthRejectsBadInput(t *testing.T) {
	t.Parallel()

	if _, err := atmosphere.RayleighOpticalDepth(0, 1013.25); !errors.Is(err, atmosphere.ErrWavelength) {
		t.Errorf("zero wavelength = %v, want ErrWavelength", err)
	}

	if _, err := atmosphere.RayleighOpticalDepth(550, 0); !errors.Is(err, atmosphere.ErrPressure) {
		t.Errorf("zero pressure = %v, want ErrPressure", err)
	}
}

// A normalised phase function integrates to exactly 1 over the sphere.
// This is the single most valuable check on any scattering kernel: it
// catches a wrong prefactor, a missing 4*pi, and a mis-transcribed
// depolarisation term at once.
func integrateOverSphere(t *testing.T, p func(theta float64) float64) float64 {
	t.Helper()

	// Gauss-free but dense: the integrands are smooth in cos(theta), so a
	// fine midpoint rule in mu = cos(theta) converges quickly.
	const n = 200000

	var sum float64

	for i := range n {
		mu := -1 + (float64(i)+0.5)*2/float64(n)
		sum += p(math.Acos(mu))
	}

	return sum * (2 / float64(n)) * 2 * math.Pi
}

func TestRayleighPhaseFunctionNormalisation(t *testing.T) {
	t.Parallel()

	for _, rho := range []float64{0, atmosphere.RayleighDepolarisation, 0.03} {
		got := integrateOverSphere(t, func(theta float64) float64 {
			return atmosphere.RayleighPhaseFunction(theta, rho)
		})

		if math.Abs(got-1) > 1e-6 {
			t.Errorf("Rayleigh phase function with rho=%v integrates to %v, want 1", rho, got)
		}
	}
}

// Winkler (2022) states that (1+3rho)/(1-rho) = 1.06 for the adopted
// depolarisation, matching the coefficient Krisciunas & Schaefer (1991)
// fitted empirically. That identity is what ties the adopted rho to the
// literature, so it is asserted directly.
func TestRayleighDepolarisationMatchesKS91Coefficient(t *testing.T) {
	t.Parallel()

	rho := atmosphere.RayleighDepolarisation

	got := (1 + 3*rho) / (1 - rho)
	if math.Abs(got-1.06) > 5e-4 {
		t.Errorf("(1+3rho)/(1-rho) = %.5f for rho=%v, want 1.06", got, rho)
	}

	// Bucholtz (1995) tabulates 0.01384 <= rho <= 0.01557 across the
	// studied range; the adopted value must lie inside it.
	if rho < 0.01384 || rho > 0.01557 {
		t.Errorf("adopted rho %v lies outside Bucholtz's tabulated range", rho)
	}
}

// Rayleigh scattering is symmetric forward and back — the physical
// signature of dipole scattering, and what distinguishes it from the
// forward-peaked aerosol term.
//
// The peak-to-side ratio is exactly 2 only for an ideal dipole. Real air
// is slightly depolarising, which lifts the 90-degree minimum and lowers
// the ratio to (1+3rho)/(1-rho) + 1 over (1+3rho)/(1-rho) = 2.06/1.06 =
// 1.9434 for the adopted rho. That is precisely the ratio implied by
// Krisciunas & Schaefer's (1991) f_R = C_R(1.06 + cos^2 theta), so the
// exact value is asserted rather than a tolerance around 2.
func TestRayleighPhaseFunctionSymmetry(t *testing.T) {
	t.Parallel()

	rho := atmosphere.RayleighDepolarisation

	fwd := atmosphere.RayleighPhaseFunction(0, rho)
	back := atmosphere.RayleighPhaseFunction(math.Pi, rho)
	side := atmosphere.RayleighPhaseFunction(math.Pi/2, rho)

	if math.Abs(fwd-back)/fwd > 1e-12 {
		t.Errorf("forward %v and backward %v scattering must be equal", fwd, back)
	}

	k := (1 + 3*rho) / (1 - rho)

	want := (k + 1) / k
	if ratio := fwd / side; math.Abs(ratio-want) > 1e-12 {
		t.Errorf("peak-to-side ratio = %v, want %v for rho=%v", ratio, want, rho)
	}

	// The ideal dipole limit must recover exactly 2.
	ideal := atmosphere.RayleighPhaseFunction(0, 0) / atmosphere.RayleighPhaseFunction(math.Pi/2, 0)
	if math.Abs(ideal-2) > 1e-12 {
		t.Errorf("rho=0 peak-to-side ratio = %v, want exactly 2", ideal)
	}
}

func TestHenyeyGreensteinNormalisation(t *testing.T) {
	t.Parallel()

	for _, g := range []float64{-0.5, 0, 0.3, 0.5, 0.85} {
		got := integrateOverSphere(t, func(theta float64) float64 {
			v, err := atmosphere.HenyeyGreensteinPhaseFunction(theta, g)
			if err != nil {
				t.Fatalf("HenyeyGreensteinPhaseFunction: %v", err)
			}

			return v
		})

		if math.Abs(got-1) > 1e-4 {
			t.Errorf("Henyey-Greenstein with g=%v integrates to %v, want 1", g, got)
		}
	}
}

// g = 0 must reduce to isotropic scattering, 1/(4*pi) in every direction.
// A positive g must be forward-peaked. These two properties define the
// parameter's meaning.
func TestHenyeyGreensteinLimits(t *testing.T) {
	t.Parallel()

	for _, theta := range []float64{0, 1, 2, math.Pi} {
		got, err := atmosphere.HenyeyGreensteinPhaseFunction(theta, 0)
		if err != nil {
			t.Fatalf("HenyeyGreensteinPhaseFunction: %v", err)
		}

		if math.Abs(got-1/(4*math.Pi)) > 1e-12 {
			t.Errorf("g=0 at theta=%v gave %v, want isotropic %v", theta, got, 1/(4*math.Pi))
		}
	}

	fwd, err := atmosphere.HenyeyGreensteinPhaseFunction(0, 0.5)
	if err != nil {
		t.Fatalf("HenyeyGreensteinPhaseFunction: %v", err)
	}

	back, err := atmosphere.HenyeyGreensteinPhaseFunction(math.Pi, 0.5)
	if err != nil {
		t.Fatalf("HenyeyGreensteinPhaseFunction: %v", err)
	}

	if fwd <= back {
		t.Errorf("g=0.5 must be forward-peaked: forward %v, backward %v", fwd, back)
	}
}

func TestHenyeyGreensteinRejectsSingularG(t *testing.T) {
	t.Parallel()

	for _, g := range []float64{-1, 1, 1.5, math.NaN()} {
		if _, err := atmosphere.HenyeyGreensteinPhaseFunction(0, g); !errors.Is(err, atmosphere.ErrAsymmetry) {
			t.Errorf("g=%v = %v, want ErrAsymmetry", g, err)
		}
	}
}

// The combined phase function must remain normalised for any mixture, and
// must reduce to each pure component at the limits — the property that
// makes Winkler Eq. 12 a weighting rather than an approximation.
func TestCombinedPhaseFunction(t *testing.T) {
	t.Parallel()

	const (
		g   = 0.5
		rho = atmosphere.RayleighDepolarisation
	)

	got := integrateOverSphere(t, func(theta float64) float64 {
		v, err := atmosphere.CombinedPhaseFunction(theta, 0.1, 0.05, g, rho)
		if err != nil {
			t.Fatalf("CombinedPhaseFunction: %v", err)
		}

		return v
	})

	if math.Abs(got-1) > 1e-4 {
		t.Errorf("combined phase function integrates to %v, want 1", got)
	}

	// Pure Rayleigh limit.
	pure, err := atmosphere.CombinedPhaseFunction(1.0, 0.1, 0, g, rho)
	if err != nil {
		t.Fatalf("CombinedPhaseFunction: %v", err)
	}

	if want := atmosphere.RayleighPhaseFunction(1.0, rho); math.Abs(pure-want) > 1e-12 {
		t.Errorf("zero aerosol gave %v, want the pure Rayleigh %v", pure, want)
	}

	// Pure aerosol limit.
	pureM, err := atmosphere.CombinedPhaseFunction(1.0, 0, 0.05, g, rho)
	if err != nil {
		t.Fatalf("CombinedPhaseFunction: %v", err)
	}

	want, err := atmosphere.HenyeyGreensteinPhaseFunction(1.0, g)
	if err != nil {
		t.Fatalf("HenyeyGreensteinPhaseFunction: %v", err)
	}

	if math.Abs(pureM-want) > 1e-12 {
		t.Errorf("zero Rayleigh gave %v, want the pure aerosol %v", pureM, want)
	}
}

func TestCombinedPhaseFunctionRejectsNoScattering(t *testing.T) {
	t.Parallel()

	if _, err := atmosphere.CombinedPhaseFunction(1, 0, 0, 0.5, 0.0148); !errors.Is(err, atmosphere.ErrOpticalDepth) {
		t.Errorf("zero total optical depth = %v, want ErrOpticalDepth", err)
	}

	if _, err := atmosphere.CombinedPhaseFunction(1, -1, 0.1, 0.5, 0.0148); !errors.Is(err, atmosphere.ErrOpticalDepth) {
		t.Errorf("negative optical depth = %v, want ErrOpticalDepth", err)
	}
}

// Transmission and optical depth are inverses, and a zenith observation
// through a standard atmosphere must transmit roughly 90 per cent at
// 550 nm — a number an observer would recognise.
func TestTransmission(t *testing.T) {
	t.Parallel()

	tau, err := atmosphere.RayleighOpticalDepth(550, atmosphere.StandardPressureHPa)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	got := atmosphere.Transmission(tau)
	if float64(got) < 0.88 || float64(got) > 0.93 {
		t.Errorf("zenith Rayleigh transmission at 550 nm = %v, want ~0.91", float64(got))
	}

	// Doubling the path halves it in log space.
	double := atmosphere.Transmission(tau * 2)
	if math.Abs(float64(double)-float64(got)*float64(got)) > 1e-12 {
		t.Errorf("two airmasses gave %v, want %v", float64(double), float64(got)*float64(got))
	}
}
