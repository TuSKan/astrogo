package skybrightness_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// Eq. 3's structure: t grows linearly with distance, falls with the
// airmass toward the source, and grows as either scale height shrinks —
// a shallower atmosphere concentrates the same optical depth into a
// shorter path, raising the extinction per unit length.
func TestKocifaj2022Eq3Structure(t *testing.T) {
	t.Parallel()

	const (
		tauA = 0.1
		hA   = 1.5 // km, one of the values the paper's corroboration uses
		tauR = 0.1
		hR   = 8.0 // km, the molecular scale height
		mS   = 2.0
	)

	base, err := skybrightness.OpticalParameterT(tauA, hA, tauR, hR, 10, mS)
	if err != nil {
		t.Fatalf("OpticalParameterT: %v", err)
	}

	// Linear in separation.
	far, err := skybrightness.OpticalParameterT(tauA, hA, tauR, hR, 20, mS)
	if err != nil {
		t.Fatalf("OpticalParameterT: %v", err)
	}

	if rel := math.Abs(far/base - 2); rel > 1e-12 {
		t.Errorf("doubling the separation scaled t by %v, want 2", far/base)
	}

	// Inverse in source airmass.
	steep, err := skybrightness.OpticalParameterT(tauA, hA, tauR, hR, 10, 2*mS)
	if err != nil {
		t.Fatalf("OpticalParameterT: %v", err)
	}

	if rel := math.Abs(steep/base - 0.5); rel > 1e-12 {
		t.Errorf("doubling the source airmass scaled t by %v, want 0.5", steep/base)
	}

	// The explicit closed form, computed independently.
	want := (tauA/hA + tauR/hR) * 10 / mS
	if rel := math.Abs(base-want) / want; rel > 1e-12 {
		t.Errorf("t = %v, want %v", base, want)
	}
}

func TestOpticalParameterTRejectsBadInput(t *testing.T) {
	t.Parallel()

	if _, err := skybrightness.OpticalParameterT(0.1, 0, 0.1, 8, 10, 2); !errors.Is(err, skybrightness.ErrScaleHeight) {
		t.Errorf("zero aerosol scale height = %v, want ErrScaleHeight", err)
	}

	if _, err := skybrightness.OpticalParameterT(0.1, 1.5, 0.1, 0, 10, 2); !errors.Is(err, skybrightness.ErrScaleHeight) {
		t.Errorf("zero molecular scale height = %v, want ErrScaleHeight", err)
	}

	if _, err := skybrightness.OpticalParameterT(0.1, 1.5, 0.1, 8, 10, 0); !errors.Is(err, skybrightness.ErrAirmass) {
		t.Errorf("zero source airmass = %v, want ErrAirmass", err)
	}
}

// The paper's own stated asymptote: as the aerosol optical thickness
// approaches zero, g tends to 0.33 regardless of the aerosol asymmetry,
// because a multiply-scattering molecular atmosphere is still anisotropic.
// This is the independent check on Eq. 5's constant term.
func TestKocifaj2022Eq4CleanAtmosphereAsymptote(t *testing.T) {
	t.Parallel()

	for _, ga := range []float64{0, 0.3, 0.6, 0.9} {
		got, err := skybrightness.AsymmetryParameter(unit.AsymmetryParameter(ga), 0)
		if err != nil {
			t.Fatalf("AsymmetryParameter(ga=%v, tau=0): %v", ga, err)
		}

		if math.Abs(got-0.33) > 1e-12 {
			t.Errorf("g at zero aerosol with g_a=%v = %v, want 0.33", ga, got)
		}
	}
}

// The explicit closed form of Eq. 4 and Eq. 5, computed independently.
func TestKocifaj2022Eq5Coefficients(t *testing.T) {
	t.Parallel()

	const (
		tau = 0.1
		ga  = 0.7
	)

	got, err := skybrightness.AsymmetryParameter(ga, tau)
	if err != nil {
		t.Fatalf("AsymmetryParameter: %v", err)
	}

	c0 := 0.33 + 0.15*tau
	c1 := 0.9 * math.Pow(tau, 0.51)
	c2 := 1.3 * math.Pow(tau, 1.85)
	want := c0 + c1*ga + c2*ga*ga

	if math.Abs(got-want) > 1e-12 {
		t.Errorf("g = %v, want %v", got, want)
	}

	// A sanity bound: for a moderate atmosphere the result must be a
	// usable Henyey-Greenstein parameter.
	if got <= 0 || got >= 1 {
		t.Errorf("g = %v, want a value inside (0, 1) for tau=%v, g_a=%v", got, tau, ga)
	}
}

// g must rise with both aerosol loading and aerosol forward-scattering,
// which is the physical content of the fit.
func TestKocifaj2022Eq4Monotonic(t *testing.T) {
	t.Parallel()

	var prev float64

	for i, tau := range []float64{0, 0.05, 0.1, 0.2} {
		got, err := skybrightness.AsymmetryParameter(0.6, unit.OpticalDepth(tau))
		if err != nil {
			t.Fatalf("AsymmetryParameter(tau=%v): %v", tau, err)
		}

		if i > 0 && got <= prev {
			t.Errorf("tau=%v gave g=%v, expected more than the previous %v", tau, got, prev)
		}

		prev = got
	}

	low, err := skybrightness.AsymmetryParameter(0.2, 0.1)
	if err != nil {
		t.Fatalf("AsymmetryParameter: %v", err)
	}

	high, err := skybrightness.AsymmetryParameter(0.8, 0.1)
	if err != nil {
		t.Fatalf("AsymmetryParameter: %v", err)
	}

	if high <= low {
		t.Errorf("g_a=0.8 gave %v, expected more than g_a=0.2's %v", high, low)
	}
}

// The published fit is not bounded to the physical range. At high aerosol
// loading with a strongly forward-scattering aerosol it exceeds 1, where a
// Henyey-Greenstein phase function is undefined. That is a limit of the
// parameterisation, and it must be reported rather than silently clamped.
func TestKocifaj2022Eq4LeavesPhysicalRange(t *testing.T) {
	t.Parallel()

	got, err := skybrightness.AsymmetryParameter(0.9, 0.5)
	if !errors.Is(err, skybrightness.ErrAsymmetryOutOfRange) {
		t.Fatalf("AsymmetryParameter(0.9, 0.5) = (%v, %v), want ErrAsymmetryOutOfRange", got, err)
	}

	if got <= 1 {
		t.Errorf("the out-of-range value %v should still be returned for diagnosis", got)
	}
}

// The paper's own stated limit: looking at the horizon in the source's
// azimuth, where the line-of-sight airmass equals the airmass toward the
// source, Eq. 2 must reduce exactly to L_S*P(g,theta)*(1-g)^2/(1+g).
//
// This holds only if the anisotropy factor, the M(z)/(M_S*t) prefactor and
// the removable singularity are all transcribed correctly — but note it does
// NOT discriminate the sign convention inside the exponential, since at
// M_S = M(z) that term is 1 either way. It is a necessary check, not a
// sufficient one, which is why the distance test below exists.
func TestKocifaj2022Eq2HorizonLimit(t *testing.T) {
	t.Parallel()

	const (
		ls   = 1.5e-3
		p    = 0.11
		g    = 0.62
		mass = 12.0
		tt   = 0.28
	)

	got, err := skybrightness.AllSkyRadiance(ls, p, g, mass, mass, tt)
	if err != nil {
		t.Fatalf("AllSkyRadiance: %v", err)
	}

	want := ls * p * (1 - g) * (1 - g) / (1 + g)

	if rel := math.Abs(got-want) / want; rel > 1e-12 {
		t.Errorf("horizon limit = %.10g, want %.10g (relative %.2e)", got, want, rel)
	}
}

// The singularity at M_S = M(z) is removable, so the kernel has to be smooth
// through it rather than merely defined at it. A bare (e^{u*t}-1)/u loses all
// precision as u approaches zero, which would show up as a notch or a spike
// in a sky map right at the source azimuth.
func TestKocifaj2022Eq2SingularityIsSmooth(t *testing.T) {
	t.Parallel()

	const (
		ls = 1.0
		p  = 0.1
		g  = 0.6
		ms = 10.0
		tt = 0.3
	)

	at := func(mz float64) float64 {
		v, err := skybrightness.AllSkyRadiance(ls, p, g, ms, mz, tt)
		if err != nil {
			t.Fatalf("AllSkyRadiance(M(z)=%v): %v", mz, err)
		}

		return v
	}

	exact := at(ms)

	// Approaching from both sides, over eleven orders of magnitude.
	for _, eps := range []float64{1e-1, 1e-3, 1e-6, 1e-9, 1e-12} {
		for _, side := range []float64{-1, 1} {
			v := at(ms + side*eps)

			if math.IsNaN(v) || v <= 0 {
				t.Fatalf("M(z) = M_S %+g gave %v", side*eps, v)
			}

			// The kernel is smooth, so the departure from the exact value
			// must shrink with eps rather than blow up.
			if rel := math.Abs(v-exact) / exact; rel > 10*eps+1e-12 {
				t.Errorf("M(z) = M_S %+g: relative departure %.3e is too large for a smooth function", side*eps, rel)
			}
		}
	}
}

// Skyglow from a ground source brightens from the zenith toward that
// source's horizon. This is the model's central directional claim — the
// reason it is an all-sky model at all — and unlike the horizon limit it
// depends on the sign of the exponent.
//
// The claim holds while the source-to-observer optical depth M_S*t stays
// below about 2, which covers the ordinary case. Beyond that the maximum
// moves off the horizon; TestKocifaj2022Eq2NearHorizonTurnover pins where.
func TestKocifaj2022Eq2BrightensTowardTheHorizon(t *testing.T) {
	t.Parallel()

	const (
		ls = 1.0
		p  = 0.1
		g  = 0.6
		ms = 15.0
		tt = 0.1 // M_S*t = 1.5, an optically thin path to the source
	)

	prev := 0.0

	// From the zenith (airmass 1) out toward the source's horizon airmass.
	for _, mz := range []float64{1, 2, 4, 8, 12, 15} {
		got, err := skybrightness.AllSkyRadiance(ls, p, g, ms, mz, tt)
		if err != nil {
			t.Fatalf("AllSkyRadiance(M(z)=%v): %v", mz, err)
		}

		if got <= prev {
			t.Errorf("M(z) = %v gave %.6g, expected more than %.6g nearer the zenith", mz, got, prev)
		}

		prev = got
	}

	// The horizon is several times brighter than the zenith, not marginally.
	zenith, err := skybrightness.AllSkyRadiance(ls, p, g, ms, 1, tt)
	if err != nil {
		t.Fatalf("AllSkyRadiance: %v", err)
	}

	if ratio := prev / zenith; ratio < 3 {
		t.Errorf("horizon/zenith = %.2f, expected a large directional contrast", ratio)
	}
}

// Where the sky is brightest is set by one number: M_S*t, the optical depth
// of the path from source to observer.
//
// Below roughly 2 the maximum sits at the source's horizon. Above it the
// maximum moves inward and the horizon is no longer the brightest direction
// — at M_S*t = 6 the peak has reached airmass 3 and the zenith itself is
// brighter than the horizon. That last regime is a very distant or very hazy
// source, where under a quarter of a per cent of the light survives the path,
// and it is worth knowing the model inverts its directional structure there
// rather than discovering it in a sky map.
//
// This documents behaviour rather than asserting the model is right about it.
func TestKocifaj2022Eq2NearHorizonTurnover(t *testing.T) {
	t.Parallel()

	const (
		p  = 0.1
		g  = 0.6
		ms = 15.0
	)

	peak := func(tt float64) (mz, value float64) {
		for i := 10; i <= 150; i++ {
			m := float64(i) / 10

			v, err := skybrightness.AllSkyRadiance(1, p, g, ms, m, tt)
			if err != nil {
				t.Fatalf("AllSkyRadiance(M(z)=%v, t=%v): %v", m, tt, err)
			}

			if v > value {
				mz, value = m, v
			}
		}

		return mz, value
	}

	// Optically thin: the horizon is the brightest direction.
	if mz, _ := peak(0.1); mz != ms {
		t.Errorf("at M_S*t = 1.5 the peak is at airmass %v, want the horizon at %v", mz, ms)
	}

	// Optically thick: the peak has moved well inside the horizon.
	mz, value := peak(0.4)
	if mz > 5 {
		t.Errorf("at M_S*t = 6 the peak is at airmass %v, expected it well inside the horizon", mz)
	}

	horizon, err := skybrightness.AllSkyRadiance(1, p, g, ms, ms, 0.4)
	if err != nil {
		t.Fatalf("AllSkyRadiance: %v", err)
	}

	if value <= horizon {
		t.Errorf("peak %.5g does not exceed the horizon value %.5g", value, horizon)
	}
}

// The test the withdrawn implementation failed, done correctly.
//
// Eq. 2 has no distance term. Distance enters through t, which Eq. 3 makes
// proportional to the source-observer separation D, and through L_S, which
// the caller must attenuate by the transmission e^{-M_S*t} of the air column
// over that separation. Vary only t, as an earlier revision of this test did,
// and the kernel grows without bound: that is a mis-specified test, not a
// broken equation.
//
// Both together must give the physically obvious answer — a city twice as far
// away contributes less sky brightness, not more.
func TestKocifaj2022Eq2FallsWithDistance(t *testing.T) {
	t.Parallel()

	const (
		emitted = 1.0  // radiance leaving the source, fixed
		p       = 0.1  // phase function
		g       = 0.6  // asymmetry parameter
		ms      = 20.0 // airmass toward a source near the horizon
		mz      = 1.0  // looking at the zenith

		// Extinction per unit distance, tau_a/H_a + tau_R/H_R from Eq. 3.
		extinctionPerKM = 0.1125
	)

	prev := math.Inf(1)

	for _, distanceKM := range []float64{5, 10, 20, 40, 80} {
		// Eq. 3: t is proportional to D at fixed airmass.
		tt := extinctionPerKM * distanceKM / ms

		// L_S as it reaches the observer, per this function's contract.
		arriving := emitted * math.Exp(-ms*tt)

		got, err := skybrightness.AllSkyRadiance(arriving, p, g, ms, mz, tt)
		if err != nil {
			t.Fatalf("AllSkyRadiance at %v km: %v", distanceKM, err)
		}

		if got >= prev {
			t.Errorf("a source at %v km contributes %.6g, more than the %.6g of the nearer one",
				distanceKM, got, prev)
		}

		prev = got
	}

	// And state the failure mode explicitly, so that a future change which
	// reintroduces it is caught here rather than in a sky map: holding the
	// source radiance fixed while distance grows is the mis-specification,
	// and it must produce the wrong sense.
	near, err := skybrightness.AllSkyRadiance(emitted, p, g, ms, mz, extinctionPerKM*20/ms)
	if err != nil {
		t.Fatalf("AllSkyRadiance: %v", err)
	}

	far, err := skybrightness.AllSkyRadiance(emitted, p, g, ms, mz, extinctionPerKM*80/ms)
	if err != nil {
		t.Fatalf("AllSkyRadiance: %v", err)
	}

	if far <= near {
		t.Error("the unattenuated kernel no longer grows with t; the contract note in " +
			"AllSkyRadiance's doc comment describes behaviour the code no longer has")
	}
}

// Scaling: the kernel is linear in the source radiance and in the phase
// function, which is what lets a whole-sky map be summed over many sources.
func TestKocifaj2022Eq2IsLinearInSourceAndPhase(t *testing.T) {
	t.Parallel()

	const (
		p  = 0.1
		g  = 0.6
		ms = 10.0
		mz = 3.0
		tt = 0.25
	)

	one, err := skybrightness.AllSkyRadiance(1, p, g, ms, mz, tt)
	if err != nil {
		t.Fatalf("AllSkyRadiance: %v", err)
	}

	ten, err := skybrightness.AllSkyRadiance(10, p, g, ms, mz, tt)
	if err != nil {
		t.Fatalf("AllSkyRadiance: %v", err)
	}

	if rel := math.Abs(ten-10*one) / (10 * one); rel > 1e-12 {
		t.Errorf("ten times the source radiance gave %.10g, want %.10g", ten, 10*one)
	}

	doubleP, err := skybrightness.AllSkyRadiance(1, 2*p, g, ms, mz, tt)
	if err != nil {
		t.Fatalf("AllSkyRadiance: %v", err)
	}

	if rel := math.Abs(doubleP-2*one) / (2 * one); rel > 1e-12 {
		t.Errorf("twice the phase function gave %.10g, want %.10g", doubleP, 2*one)
	}
}

func TestKocifaj2022Eq2RejectsBadInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                 string
		ls, p, g, ms, mz, tt float64
		want                 error
	}{
		{"negative radiance", -1, 0.1, 0.5, 5, 2, 0.2, skybrightness.ErrNegativeRadiance},
		{"asymmetry at one", 1, 0.1, 1, 5, 2, 0.2, skybrightness.ErrAsymmetryRange},
		{"asymmetry below minus one", 1, 0.1, -1.5, 5, 2, 0.2, skybrightness.ErrAsymmetryRange},
		{"source airmass below one", 1, 0.1, 0.5, 0.5, 2, 0.2, skybrightness.ErrAirmass},
		{"view airmass below one", 1, 0.1, 0.5, 5, 0, 0.2, skybrightness.ErrAirmass},
		{"zero t", 1, 0.1, 0.5, 5, 2, 0, skybrightness.ErrOpticalParameter},
		{"negative t", 1, 0.1, 0.5, 5, 2, -0.1, skybrightness.ErrOpticalParameter},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := skybrightness.AllSkyRadiance(tc.ls, tc.p, tc.g, tc.ms, tc.mz, tc.tt)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func BenchmarkAllSkyRadiance(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := skybrightness.AllSkyRadiance(1e-3, 0.1, 0.6, 12, 3, 0.25); err != nil {
			b.Fatal(err)
		}
	}
}

// Kocifaj, Bará & Falchi (2022) Fig. 1 — the paper's own Level-2 validation
// target: the effective asymmetry parameter g against the aerosol asymmetry
// parameter g_a, for three aerosol optical depths.
//
// The figure itself is not digitised here, so this checks the properties the
// paper states about it rather than pixel values: all three curves start at
// 0.33 + 0.15*tau_a when the aerosol is isotropic, rise monotonically with
// g_a, and separate in tau_a order across the whole range. A digitised
// comparison would be strictly better and is what §13's Level 2 ultimately
// wants; this is what can be asserted from the text.
func TestKocifaj2022Fig1Curves(t *testing.T) {
	t.Parallel()

	// The paper's own corroboration used MSOS at 450 and 550 nm with g_a
	// from 0 to 0.9.
	aerosolAsymmetry := []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9}
	depths := []float64{0.05, 0.1, 0.2}

	curves := make([][]float64, len(depths))

	for d, tau := range depths {
		curve := make([]float64, len(aerosolAsymmetry))

		for i, ga := range aerosolAsymmetry {
			g, err := skybrightness.AsymmetryParameter(
				unit.AsymmetryParameter(ga), unit.OpticalDepth(tau))
			if err != nil {
				t.Fatalf("AsymmetryParameter(g_a=%v, tau=%v): %v", ga, tau, err)
			}

			curve[i] = g
		}

		curves[d] = curve
	}

	for d, tau := range depths {
		curve := curves[d]

		// The isotropic-aerosol intercept is Eq. 5's c0 exactly.
		if want := 0.33 + 0.15*tau; math.Abs(curve[0]-want) > 1e-12 {
			t.Errorf("tau_a = %v: g at g_a = 0 is %v, want c0 = %v", tau, curve[0], want)
		}

		for i := 1; i < len(curve); i++ {
			if curve[i] <= curve[i-1] {
				t.Errorf("tau_a = %v: g fell from %v to %v between g_a %v and %v",
					tau, curve[i-1], curve[i], aerosolAsymmetry[i-1], aerosolAsymmetry[i])
			}
		}

		// Every point stays a usable Henyey-Greenstein parameter over the
		// range the paper plots.
		for i, g := range curve {
			if g <= 0 || g >= 1 {
				t.Errorf("tau_a = %v, g_a = %v: g = %v is outside (0, 1)",
					tau, aerosolAsymmetry[i], g)
			}
		}
	}

	// The curves are ordered by optical depth and do not cross.
	for i := range aerosolAsymmetry {
		if !(curves[0][i] < curves[1][i] && curves[1][i] < curves[2][i]) {
			t.Errorf("at g_a = %v the tau_a curves are out of order: %v, %v, %v",
				aerosolAsymmetry[i], curves[0][i], curves[1][i], curves[2][i])
		}
	}
}
