package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// At 550 nm and sea level the Rayleigh optical depth is the familiar
// ~0.097: Hansen & Travis (1974) print 0.0973 at 0.55 µm, an independent
// formulation from Bodhaine et al.'s (1999) that the function implements.
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

// bodhaineTable3 is rows of Bodhaine et al. (1999) Table 3: the Rayleigh
// optical depth computed exactly, not fitted, for dry air with 360 ppm CO2,
// at sea level and 45° (1013.25 mb) and at Mauna Loa (680 mb, 19.533°N,
// 3400 m), copied from the published table.
var bodhaineTable3 = []struct {
	nm, seaLevel, mlo float64
}{
	{250, 2.7137, 1.8264},
	{300, 1.2164, 0.81868},
	{330, 0.80873, 0.54429},
	{350, 0.63031, 0.42421},
	{400, 0.36022, 0.24243},
	{450, 0.22111, 0.14881},
	{550, 0.097069, 0.065329},
	{700, 0.036424, 0.024514},
	{850, 0.016621, 0.011186},
	{1000, 0.0086366, 0.0058125},
}

// TestRayleighOpticalDepthIsBodhainesTable3 is #592. RayleighOpticalDepth was
// Winkler's (2022) single power law, λ^-4.05, which read 4.2% low at 330 nm
// because the exponent steepens toward the blue. It is Bodhaine et al.'s
// (1999) Eq. 30 now, held here to the exact values their Table 3 tabulates,
// which the fit reproduces to 0.05% from 250 to 1000 nm.
func TestRayleighOpticalDepthIsBodhainesTable3(t *testing.T) {
	t.Parallel()

	for _, row := range bodhaineTable3 {
		got, err := atmosphere.RayleighOpticalDepth(unit.WavelengthNM(row.nm), 1013.25)
		if err != nil {
			t.Fatalf("RayleighOpticalDepth(%g nm): %v", row.nm, err)
		}

		if rel := float64(got)/row.seaLevel - 1; math.Abs(rel) > 5e-4 {
			t.Errorf("tau_R(%g nm, 1013.25 hPa) = %.6g, Bodhaine Table 3 has %.5g (%+.4f%%)",
				row.nm, float64(got), row.seaLevel, 100*rel)
		}
	}
}

// TestRayleighOpticalDepthPressureScalingAtMaunaLoa: scaling the 45°,
// sea-level column by pressure leaves gravity at its 45°, sea-level value.
// Bodhaine's Mauna Loa column, at 19.5°N and 3.4 km where gravity is lower,
// is 0.29% larger than that scaling, which bounds what it leaves out.
func TestRayleighOpticalDepthPressureScalingAtMaunaLoa(t *testing.T) {
	t.Parallel()

	for _, row := range bodhaineTable3 {
		got, err := atmosphere.RayleighOpticalDepth(unit.WavelengthNM(row.nm), 680)
		if err != nil {
			t.Fatalf("RayleighOpticalDepth(%g nm): %v", row.nm, err)
		}

		if rel := float64(got)/row.mlo - 1; rel > 0 || rel < -0.004 {
			t.Errorf("tau_R(%g nm, 680 hPa) = %.6g, Bodhaine's Mauna Loa column is %.5g (%+.3f%%), "+
				"want at most 0.4%% below it", row.nm, float64(got), row.mlo, 100*rel)
		}
	}
}

// TestRayleighOpticalDepthBeyondTheFit: past 1000 nm, where Eq. 30 was not
// fitted and its rational form drifts from λ^-4, it continues as λ^-4 from
// its 1000 nm value.
func TestRayleighOpticalDepthBeyondTheFit(t *testing.T) {
	t.Parallel()

	at1um, err := atmosphere.RayleighOpticalDepth(1000, 1013.25)
	if err != nil {
		t.Fatal(err)
	}

	at2um, err := atmosphere.RayleighOpticalDepth(2000, 1013.25)
	if err != nil {
		t.Fatal(err)
	}

	if got := float64(at1um / at2um); math.Abs(got-16) > 1e-9 {
		t.Errorf("tau(1000 nm)/tau(2000 nm) = %v, want 16", got)
	}
}

func TestRayleighOpticalDepthRejectsBadInput(t *testing.T) {
	t.Parallel()

	if _, err := atmosphere.RayleighOpticalDepth(0, 1013.25); !errors.Is(err, atmosphere.ErrWavelength) {
		t.Errorf("zero wavelength = %v, want ErrWavelength", err)
	}

	// Below the fit's 250 nm, where its denominator heads for a zero near
	// 117 nm.
	if _, err := atmosphere.RayleighOpticalDepth(249, 1013.25); !errors.Is(err, atmosphere.ErrWavelength) {
		t.Errorf("249 nm = %v, want ErrWavelength", err)
	}

	if _, err := atmosphere.RayleighOpticalDepth(550, 0); !errors.Is(err, atmosphere.ErrPressure) {
		t.Errorf("zero pressure = %v, want ErrPressure", err)
	}
}

// A normalized phase function integrates to exactly 1 over the sphere.
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

func TestRayleighPhaseFunctionNormalization(t *testing.T) {
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

func TestHenyeyGreensteinNormalization(t *testing.T) {
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

// The combined phase function must remain normalized for any mixture, and
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
// 550 nm — a number an observer would recognize.
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
