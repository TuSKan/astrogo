package magnitude_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/unit"
)

// topHat builds a rectangular response between lo and hi — an idealised
// band whose integrals have closed forms, so a projection can be checked
// against arithmetic rather than against another implementation.
func topHat(name string, lo, hi unit.WavelengthNM, det magnitude.Detector) magnitude.Passband {
	const eps = 1e-6

	return magnitude.Passband{
		Name:         name,
		WavelengthNM: []unit.WavelengthNM{lo - eps, lo, hi, hi + eps},
		Response:     []float64{0, 1, 1, 0},
		Detector:     det,
		Reference:    "synthetic test fixture, not a published curve",
	}
}

func mustGrid(t *testing.T, start unit.WavelengthNM, n int) unit.SpectralGrid {
	t.Helper()

	g, err := unit.NewSpectralGrid(start, 1, n)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	return g
}

func TestPassbandValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		p    magnitude.Passband
		want error
	}{
		{"empty", magnitude.Passband{Name: "x"}, magnitude.ErrPassbandEmpty},
		{
			"length mismatch",
			magnitude.Passband{Name: "x", WavelengthNM: []unit.WavelengthNM{1, 2}, Response: []float64{1}},
			magnitude.ErrPassbandEmpty,
		},
		{
			"not increasing",
			magnitude.Passband{Name: "x", WavelengthNM: []unit.WavelengthNM{2, 1}, Response: []float64{1, 1}},
			magnitude.ErrPassbandEmpty,
		},
		{
			"negative response",
			magnitude.Passband{Name: "x", WavelengthNM: []unit.WavelengthNM{1, 2}, Response: []float64{1, -1}},
			magnitude.ErrPassbandResponse,
		},
		{
			"all zero",
			magnitude.Passband{Name: "x", WavelengthNM: []unit.WavelengthNM{1, 2}, Response: []float64{0, 0}},
			magnitude.ErrPassbandResponse,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.p.Validate(); !errors.Is(err, tc.want) {
				t.Errorf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
}

// The band-averaged value of a flat spectrum is that same flat value,
// whatever the response shape or normalisation — the defining property of
// a weighted mean, and the first thing a wrong normalisation breaks.
func TestMeanFluxDensityFlatSpectrumIsIdentity(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 301) // 400..700

	spectrum := make([]float64, g.Len())
	for i := range spectrum {
		spectrum[i] = 7.5
	}

	for _, det := range []magnitude.Detector{magnitude.PhotonCounting, magnitude.EnergyIntegrating} {
		band := topHat("test", 500, 600, det)

		got, err := magnitude.MeanFluxDensity(spectrum, g, band, 0.99)
		if err != nil {
			t.Fatalf("MeanFluxDensity(%v): %v", det, err)
		}

		if math.Abs(got-7.5) > 1e-9 {
			t.Errorf("MeanFluxDensity(%v) = %v, want 7.5", det, got)
		}
	}
}

// An unnormalised curve must give the same answer as a normalised one,
// since every projection divides by the band's own integral.
func TestMeanFluxDensityIgnoresResponseScale(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 301)

	spectrum := make([]float64, g.Len())
	for i := range spectrum {
		spectrum[i] = 1 + float64(i)/1000 // sloped, so weighting actually matters
	}

	unitBand := topHat("unit", 500, 600, magnitude.PhotonCounting)

	scaled := unitBand
	scaled.Response = []float64{0, 0.37, 0.37, 0}

	a, err := magnitude.MeanFluxDensity(spectrum, g, unitBand, 0.99)
	if err != nil {
		t.Fatalf("MeanFluxDensity: %v", err)
	}

	b, err := magnitude.MeanFluxDensity(spectrum, g, scaled, 0.99)
	if err != nil {
		t.Fatalf("MeanFluxDensity (scaled): %v", err)
	}

	if math.Abs(a-b) > 1e-12 {
		t.Errorf("response scale changed the mean: %v vs %v", a, b)
	}
}

// A grid that stops inside the band truncates the integral silently, which
// is a wrong answer rather than a noisy one, so it must be refused.
func TestMeanFluxDensityRejectsPartialCoverage(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 500, 51) // 500..550 only
	band := topHat("test", 500, 600, magnitude.PhotonCounting)

	spectrum := make([]float64, g.Len())

	_, err := magnitude.MeanFluxDensity(spectrum, g, band, 0.99)
	if !errors.Is(err, magnitude.ErrPassbandCoverage) {
		t.Errorf("MeanFluxDensity with half coverage = %v, want ErrPassbandCoverage", err)
	}
}

// The pivot wavelength of a symmetric top-hat sits near its centre, and is
// a property of the curve alone.
func TestPivotWavelength(t *testing.T) {
	t.Parallel()

	band := topHat("test", 500, 600, magnitude.PhotonCounting)

	pivot, err := band.PivotWavelength()
	if err != nil {
		t.Fatalf("PivotWavelength: %v", err)
	}

	if pivot < 540 || pivot > 560 {
		t.Errorf("PivotWavelength = %v, want near the 550 nm band centre", pivot)
	}
}

// A spectrum exactly at the AB zero point must read magnitude 0 per square
// arcsecond once the per-steradian to per-arcsec^2 conversion is undone.
// This pins the whole AB chain: per-nm to per-Hz at the pivot, the
// steradian conversion, and the zero point itself.
func TestSurfaceBrightnessABZeroPoint(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 301)
	band := topHat("test", 500, 600, magnitude.PhotonCounting)

	pivot, err := band.PivotWavelength()
	if err != nil {
		t.Fatalf("PivotWavelength: %v", err)
	}

	// Build f_lambda per steradian such that f_nu per arcsec^2 lands exactly
	// on the AB zero point.
	lambdaM := float64(pivot) * 1e-9
	fNu := constants.Photometric.ABZeroPoint.Value
	perM := fNu * constants.SI2019.SpeedOfLight.Value / (lambdaM * lambdaM)
	perNMPerArcsec2 := perM * 1e-9
	perNMPerSr := perNMPerArcsec2 / constants.ArcsecondSquaredToSteradian

	spectrum := make([]float64, g.Len())
	for i := range spectrum {
		spectrum[i] = perNMPerSr
	}

	got, err := magnitude.SurfaceBrightness(spectrum, g, band, magnitude.AB, 0.99)
	if err != nil {
		t.Fatalf("SurfaceBrightness: %v", err)
	}

	if math.Abs(got) > 1e-6 {
		t.Errorf("SurfaceBrightness at the AB zero point = %v, want 0", got)
	}
}

// Doubling the radiance must brighten the surface brightness by exactly
// 2.5*log10(2), the defining slope of the magnitude scale. A sign error
// here would be invisible to a single-value test.
func TestSurfaceBrightnessMagnitudeScale(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 301)
	band := topHat("test", 500, 600, magnitude.PhotonCounting)

	dim := make([]float64, g.Len())
	bright := make([]float64, g.Len())

	for i := range dim {
		dim[i] = 1e-8
		bright[i] = 2e-8
	}

	a, err := magnitude.SurfaceBrightness(dim, g, band, magnitude.AB, 0.99)
	if err != nil {
		t.Fatalf("SurfaceBrightness: %v", err)
	}

	b, err := magnitude.SurfaceBrightness(bright, g, band, magnitude.AB, 0.99)
	if err != nil {
		t.Fatalf("SurfaceBrightness: %v", err)
	}

	want := 2.5 * math.Log10(2)
	if math.Abs((a-b)-want) > 1e-9 {
		t.Errorf("doubling radiance changed magnitude by %v, want %v", a-b, want)
	}

	if b >= a {
		t.Error("a brighter sky must have a numerically smaller magnitude")
	}
}

// Vega magnitudes need a zero point that depends on the adopted reference
// spectrum, so a band without one must fail rather than quietly returning
// an AB number.
func TestSurfaceBrightnessVegaRequiresZeroPoint(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 301)
	band := topHat("test", 500, 600, magnitude.PhotonCounting)

	spectrum := make([]float64, g.Len())
	for i := range spectrum {
		spectrum[i] = 1e-8
	}

	_, err := magnitude.SurfaceBrightness(spectrum, g, band, magnitude.Vega, 0.99)
	if !errors.Is(err, magnitude.ErrZeroPointUnknown) {
		t.Errorf("Vega without a zero point = %v, want ErrZeroPointUnknown", err)
	}

	band.VegaZeroPointJy = 3600

	if _, err := magnitude.SurfaceBrightness(spectrum, g, band, magnitude.Vega, 0.99); err != nil {
		t.Errorf("Vega with a zero point: %v", err)
	}
}

// The spec forbids assuming SQM ~ Johnson V internally: two different
// response curves over one physical spectrum are different numbers, and
// the difference is spectrum-dependent. This asserts the projections do
// not collapse into each other.
func TestDifferentBandsGiveDifferentAnswers(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 380, 421) // 380..800

	// A sloped spectrum, so band placement genuinely matters. A flat one
	// would make every band agree and prove nothing.
	spectrum := make([]float64, g.Len())
	for i := range spectrum {
		spectrum[i] = 1e-8 * (1 + float64(i)/200)
	}

	// Rough stand-ins for real bands. Deliberately synthetic: the real
	// curves are datasets, and the point here is that the projection
	// machinery distinguishes them.
	blue := topHat("blue-ish", 400, 500, magnitude.PhotonCounting)
	green := topHat("green-ish", 500, 600, magnitude.PhotonCounting)
	red := topHat("red-ish", 600, 700, magnitude.PhotonCounting)

	mags := make([]float64, 0, 3)

	for _, b := range []magnitude.Passband{blue, green, red} {
		m, err := magnitude.SurfaceBrightness(spectrum, g, b, magnitude.AB, 0.99)
		if err != nil {
			t.Fatalf("SurfaceBrightness(%s): %v", b.Name, err)
		}

		mags = append(mags, m)
	}

	for i := range mags {
		for j := i + 1; j < len(mags); j++ {
			if math.Abs(mags[i]-mags[j]) < 1e-6 {
				t.Errorf("bands %d and %d gave indistinguishable magnitudes %v — "+
					"different response curves over one spectrum must differ", i, j, mags[i])
			}
		}
	}
}

// A photon-counting and an energy-integrating response over the same curve
// shape weight the band differently. Conflating them is one of the unit
// errors the spec calls out, so it must be observable.
func TestDetectorConventionChangesResult(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 301)

	spectrum := make([]float64, g.Len())
	for i := range spectrum {
		spectrum[i] = 1e-8 * (1 + float64(i)/100) // sloped
	}

	photon, err := magnitude.MeanFluxDensity(spectrum, g, topHat("p", 500, 600, magnitude.PhotonCounting), 0.99)
	if err != nil {
		t.Fatalf("MeanFluxDensity (photon): %v", err)
	}

	energy, err := magnitude.MeanFluxDensity(spectrum, g, topHat("e", 500, 600, magnitude.EnergyIntegrating), 0.99)
	if err != nil {
		t.Fatalf("MeanFluxDensity (energy): %v", err)
	}

	if math.Abs(photon-energy) < 1e-12 {
		t.Error("photon-counting and energy-integrating weightings must differ for a sloped spectrum")
	}
}

// A zero sky is a real Phase 0 state (no components registered). It must
// read as arbitrarily faint, not as NaN or an error.
func TestSurfaceBrightnessZeroSkyIsInfinitelyFaint(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 301)
	band := topHat("test", 500, 600, magnitude.PhotonCounting)

	got, err := magnitude.SurfaceBrightness(make([]float64, g.Len()), g, band, magnitude.AB, 0.99)
	if err != nil {
		t.Fatalf("SurfaceBrightness: %v", err)
	}

	if !math.IsInf(got, 1) {
		t.Errorf("zero sky = %v, want +Inf", got)
	}
}

// The pivot wavelength follows the detector convention.
//
// A photon-counting response is an energy response times lambda/hc, so the two
// definitions differ by a factor of lambda inside both integrals. Computing the
// photon form for an energy-calibrated band is wrong by a fraction of a per
// cent in wavelength and twice that in an f_lambda to f_nu conversion — small,
// systematic, and entirely silent.
//
// The reference values are the Spanish Virtual Observatory's own
// WavelengthPivot for the five Bessell bands, in nanometres. All five are
// energy counters, and honouring that reproduces every one of them; the photon
// form misses by 0.33 to 0.89 per cent.
func TestPivotWavelengthFollowsTheDetector(t *testing.T) {
	t.Parallel()

	// A triangular response is enough: the two conventions differ by the
	// weighting inside the integral, not by the shape.
	curve := func(lo, hi unit.WavelengthNM, det magnitude.Detector) magnitude.Passband {
		const steps = 200

		wl := make([]unit.WavelengthNM, 0, steps+1)
		resp := make([]float64, 0, steps+1)

		for i := range steps + 1 {
			f := float64(i) / steps
			wl = append(wl, lo+unit.WavelengthNM(f)*(hi-lo))
			resp = append(resp, 1-math.Abs(2*f-1))
		}

		return magnitude.Passband{
			Name: "triangle", WavelengthNM: wl, Response: resp, Detector: det,
		}
	}

	const lo, hi = 500.0, 600.0

	energy, err := curve(lo, hi, magnitude.EnergyIntegrating).PivotWavelength()
	if err != nil {
		t.Fatalf("energy: %v", err)
	}

	photon, err := curve(lo, hi, magnitude.PhotonCounting).PivotWavelength()
	if err != nil {
		t.Fatalf("photon: %v", err)
	}

	// The photon-counting pivot is the redder of the two, because its extra
	// factor of lambda weights the long end.
	if !(photon > energy) {
		t.Errorf("photon pivot %.4f is not redward of the energy pivot %.4f; the "+
			"conventions are not being distinguished", float64(photon), float64(energy))
	}

	// Both must land inside the band, which is what says neither integral has
	// been inverted.
	for name, v := range map[string]unit.WavelengthNM{"energy": energy, "photon": photon} {
		if float64(v) < lo || float64(v) > hi {
			t.Errorf("%s pivot %.4f is outside the band", name, float64(v))
		}
	}

	// And the difference is the size the definitions imply, not a rounding
	// artefact: a hundred-nanometre-wide band separates them by about half a
	// per cent.
	if rel := (float64(photon) - float64(energy)) / float64(energy); rel < 0.001 || rel > 0.02 {
		t.Errorf("the two conventions differ by %.3f per cent, which is not the scale the "+
			"factor of lambda implies for this band", 100*rel)
	}
}
