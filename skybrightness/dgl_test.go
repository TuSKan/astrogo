package skybrightness_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// The evidence for reading Kawara's quadratic coefficient as 1e-5 rather than
// the printed 1e5, made executable.
//
// The quadratic turns over at I_100 = b/(2c). Kawara fit samples bounded at
// 5, 10, 15, 20, 30 and 50 MJy/sr, and Masana et al. restrict the relation to
// I_100 < 50. A quadratic fitted to saturating data should therefore turn
// over near the top of that range, and with 1e-5 every well-measured band
// does. With the printed 1e5 the turnover would sit at 1e-9 MJy/sr and DGL
// would be negative over the whole sky.
func TestDGLTurnoverMatchesTheFittedRange(t *testing.T) {
	t.Parallel()

	// The six bands whose coefficients are measured well; the two bluest
	// have quadratic coefficients consistent with zero and so turn over far
	// outside the range.
	for _, lambda := range []unit.WavelengthNM{319, 369, 418, 472, 550, 648} {
		b, c, extrapolated := skybrightness.DGLCoefficientsAt(lambda)
		if extrapolated {
			t.Fatalf("%v nm reported as extrapolated but is inside the table", lambda)
		}

		turnover := b / (2 * c)

		if turnover < 30 || turnover > 60 {
			t.Errorf("%v nm turns over at %.1f MJy/sr, want the 30-60 range that brackets "+
				"Kawara's own fitting bound of 50", lambda, turnover)
		}
	}
}

// A hand-computed value at 550 nm, going through the tabulated slope rather
// than the implementation's own conversion.
//
// Kawara tabulates nu*b = 20.1 nW m^-2 sr^-1 per MJy sr^-1 at 550 nm, so for
// a dust column of 1 MJy/sr past the offset the linear term alone gives
// nu*I_nu = 20.1 nW m^-2 sr^-1, less a quadratic correction of about 0.24.
// Since nu*I_nu = lambda*I_lambda, that pins the spectral radiance directly.
func TestDGLAgainstTabulatedSlope(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(550, 1, 2)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	dst := skybrightness.NewSpectralRadiance(grid)

	// One MJy/sr of dust, plus the offset that gets subtracted back off.
	if _, err := skybrightness.DiffuseGalacticLight(dst, grid, 1+skybrightness.DustEmissionOffsetMJy); err != nil {
		t.Fatalf("DiffuseGalacticLight: %v", err)
	}

	// nu*b*I - nu*c*I^2, in nW m^-2 sr^-1, with nu*c = (3000/0.55)*4.4e-5.
	const nuInu = 20.1 - 5454.5454545*4.4e-5

	// lambda * I_lambda = nu * I_nu, so I_lambda = nu*I_nu / lambda.
	want := nuInu * 1e-9 / 550

	if rel := math.Abs(dst[0]-want) / want; rel > 1e-3 {
		t.Errorf("I_lambda(550 nm) = %.6g, want %.6g W m^-2 sr^-1 nm^-1 (relative %.2e)",
			dst[0], want, rel)
	}
}

// Below the extragalactic offset there is no dust to scatter from, and the
// answer must be exactly zero rather than a small negative radiance.
func TestDGLBelowTheOffsetIsZero(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()

	for _, sfd := range []float64{0, 0.4, skybrightness.DustEmissionOffsetMJy} {
		dst := skybrightness.NewSpectralRadiance(grid)

		if _, err := skybrightness.DiffuseGalacticLight(dst, grid, sfd); err != nil {
			t.Fatalf("DiffuseGalacticLight(%v): %v", sfd, err)
		}

		for i, v := range dst {
			if v != 0 {
				t.Fatalf("SFD %v MJy/sr: band %d is %v, want 0", sfd, i, v)
			}
		}
	}
}

// In the optically thin regime the relation is very nearly linear in the dust
// column, which is the physical claim the correlation rests on.
func TestDGLIsNearlyLinearWhenThin(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(550, 1, 2)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	at := func(dust float64) float64 {
		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := skybrightness.DiffuseGalacticLight(dst, grid, dust+skybrightness.DustEmissionOffsetMJy); err != nil {
			t.Fatalf("DiffuseGalacticLight: %v", err)
		}

		return dst[0]
	}

	one := at(0.1)
	ten := at(1.0)

	// Ten times the dust, within a per cent of ten times the light.
	if rel := math.Abs(ten-10*one) / (10 * one); rel > 0.02 {
		t.Errorf("ten times the dust gave %.6g against %.6g, a %.1f%% departure from linear",
			ten, 10*one, rel*100)
	}
}

// The correlation saturates: DGL rises, peaks near the top of the fitted
// range, and the model refuses to report the negative values the bare
// quadratic would give beyond it.
func TestDGLSaturates(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(550, 1, 2)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	at := func(sfd float64) float64 {
		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := skybrightness.DiffuseGalacticLight(dst, grid, sfd); err != nil {
			t.Fatalf("DiffuseGalacticLight(%v): %v", sfd, err)
		}

		return dst[0]
	}

	var peak, peakAt float64

	for i := 1; i <= 200; i++ {
		sfd := float64(i)
		if v := at(sfd); v > peak {
			peak, peakAt = v, sfd
		}
	}

	if peakAt < 30 || peakAt > 60 {
		t.Errorf("DGL peaks at %v MJy/sr, want near the fitted bound of 50", peakAt)
	}

	// Far past the turnover the bare quadratic is negative; the clamp must
	// hold and the result must be flagged rather than silently zero.
	dst := skybrightness.NewSpectralRadiance(grid)

	flags, err := skybrightness.DiffuseGalacticLight(dst, grid, 500)
	if err != nil {
		t.Fatalf("DiffuseGalacticLight: %v", err)
	}

	if dst[0] != 0 {
		t.Errorf("far past the turnover gave %v, want the clamp to hold at 0", dst[0])
	}

	if flags&skybrightness.ExtrapolatedModel == 0 {
		t.Error("leaving the fitted regime was not flagged")
	}
}

// The coefficients rise with wavelength across the tabulated range — DGL is
// redder than the starlight scattering into it, because dust scatters blue
// light out of the beam more efficiently.
func TestDGLCoefficientsRiseWithWavelength(t *testing.T) {
	t.Parallel()

	var prev float64

	for _, lambda := range []unit.WavelengthNM{225, 274, 319, 369, 418, 472, 550, 648} {
		b, _, _ := skybrightness.DGLCoefficientsAt(lambda)

		if b <= prev {
			t.Errorf("slope at %v nm is %v, not above the %v at the previous band", lambda, b, prev)
		}

		prev = b
	}
}

// Outside 225-648 nm the endpoints are held, not extrapolated, and the caller
// is told. The red end matters most: the default grid runs to 1000 nm, the
// table stops at 648, and the slope is still rising there.
func TestDGLCoefficientsClampOutsideTheTable(t *testing.T) {
	t.Parallel()

	redB, redC, redExtrap := skybrightness.DGLCoefficientsAt(648)

	for _, lambda := range []unit.WavelengthNM{700, 850, 1000} {
		b, c, extrapolated := skybrightness.DGLCoefficientsAt(lambda)

		if !extrapolated {
			t.Errorf("%v nm is past the table but was not flagged", lambda)
		}

		if b != redB || c != redC {
			t.Errorf("%v nm gave (%v, %v), want the 648 nm endpoint (%v, %v) held", lambda, b, c, redB, redC)
		}
	}

	if redExtrap {
		t.Error("648 nm is the table's own endpoint and must not be flagged")
	}

	if _, _, extrapolated := skybrightness.DGLCoefficientsAt(200); !extrapolated {
		t.Error("200 nm is below the table but was not flagged")
	}
}

// Evaluating over the default grid must flag the extrapolation, because most
// of that grid lies past Kawara's red limit.
func TestDGLOnTheDefaultGridFlagsExtrapolation(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	flags, err := skybrightness.DiffuseGalacticLight(dst, grid, 5)
	if err != nil {
		t.Fatalf("DiffuseGalacticLight: %v", err)
	}

	if flags&skybrightness.ExtrapolatedModel == 0 {
		t.Error("the default grid runs past 648 nm but the result was not flagged")
	}

	for i, v := range dst {
		if v < 0 || math.IsNaN(v) {
			t.Fatalf("band %d (%v nm) is %v", i, grid.At(i), v)
		}
	}
}

// Accumulation, not assignment — DGL is one term of a sum.
func TestDGLAccumulates(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(550, 1, 2)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	once := skybrightness.NewSpectralRadiance(grid)
	if _, err := skybrightness.DiffuseGalacticLight(once, grid, 5); err != nil {
		t.Fatalf("DiffuseGalacticLight: %v", err)
	}

	twice := skybrightness.NewSpectralRadiance(grid)
	for range 2 {
		if _, err := skybrightness.DiffuseGalacticLight(twice, grid, 5); err != nil {
			t.Fatalf("DiffuseGalacticLight: %v", err)
		}
	}

	if rel := math.Abs(twice[0]-2*once[0]) / (2 * once[0]); rel > 1e-12 {
		t.Errorf("two calls gave %v, want twice the %v of one", twice[0], once[0])
	}
}

func TestDGLRejectsBadInput(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()

	if _, err := skybrightness.DiffuseGalacticLight(make(skybrightness.SpectralRadiance, 3), grid, 5); !errors.Is(err, unit.ErrGridMismatch) {
		t.Errorf("short destination: err = %v, want ErrGridMismatch", err)
	}

	dst := skybrightness.NewSpectralRadiance(grid)

	for _, sfd := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := skybrightness.DiffuseGalacticLight(dst, grid, sfd); !errors.Is(err, skybrightness.ErrDustIntensity) {
			t.Errorf("SFD %v: err = %v, want ErrDustIntensity", sfd, err)
		}
	}
}

func BenchmarkDiffuseGalacticLight(b *testing.B) {
	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := skybrightness.DiffuseGalacticLight(dst, grid, 5); err != nil {
			b.Fatal(err)
		}
	}
}
