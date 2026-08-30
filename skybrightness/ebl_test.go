package skybrightness

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// eblScene builds a scene the component can be evaluated in.
func eblScene(t *testing.T) *Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(41.38), angle.Deg(2.11), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(1013, 288).
		Aerosol(0.056, 550, 1.3, 0.9, 0.7).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	return &Scene{
		Observer:   loc,
		Time:       time.GoDate(2026, 8, 20, 23, 16, 0, 0, time.LocationUTC),
		Atmosphere: atm,
	}
}

// The published table, guarded value by value.
//
// These are the numbers the component is, and a typo in one of them is not
// visible in any downstream result — the extragalactic background is about one
// per cent of the sky, so a wrong digit changes a total by nothing measurable
// while making the component wrong. Every other test here checks behaviour;
// this one checks transcription, against Koushan et al. (2021) Table 3 with
// pivot wavelengths from Table 1.
func TestKoushanTableIsTranscribedCorrectly(t *testing.T) {
	t.Parallel()

	want := []struct {
		band  string
		pivot float64
		nuInu float64
		err   float64
	}{
		{"u", 357.7, 4.13, 6.87},
		{"g", 474.4, 5.76, 4.02},
		{"r", 631.2, 8.11, 4.08},
		{"i", 758.4, 9.94, 4.44},
		{"Z", 883.3, 10.71, 5.20},
		{"Y", 1022.4, 11.58, 4.52},
		{"J", 1254.6, 11.22, 4.92},
		{"H", 1647.7, 11.17, 4.73},
		{"Ks", 2154.9, 9.42, 5.21},
	}

	if len(koushanIGL) != len(want) {
		t.Fatalf("table has %d bands, want %d", len(koushanIGL), len(want))
	}

	for i, w := range want {
		got := koushanIGL[i]
		if got.Band != w.band || got.PivotNM != w.pivot ||
			got.NuInu != w.nuInu || got.TotalErrorPercent != w.err {
			t.Errorf("row %d = %+v, want %s %.1f nm %.2f %.2f%%",
				i, got, w.band, w.pivot, w.nuInu, w.err)
		}
	}

	// Pivot wavelengths must ascend, or the interpolation search is wrong.
	for i := 1; i < len(koushanIGL); i++ {
		if koushanIGL[i].PivotNM <= koushanIGL[i-1].PivotNM {
			t.Errorf("pivot wavelengths are not ascending at row %d", i)
		}
	}
}

// nu*I_nu is not a spectral radiance, and treating it as one is an error of
// several hundred.
//
// The table is in nW m^-2 sr^-1, an integrated surface brightness over the
// band. Spectral radiance is that divided by the pivot wavelength and scaled
// out of nanowatts. Checked against the r band worked by hand: 8.11e-9 / 631.2
// = 1.2849e-11 W m^-2 sr^-1 nm^-1, which is 27.70 mag arcsec^-2 through Johnson
// V's zero point — a plausible extragalactic background, and about a hundredth
// of this package's own integrated starlight at 22.8.
func TestExtragalacticUnitConversion(t *testing.T) {
	t.Parallel()

	e := NewExtragalacticBackground()

	got, extrapolated := e.at(631.2)
	if extrapolated {
		t.Error("a tabulated pivot wavelength reported as extrapolated")
	}

	const want = 8.11e-9 / 631.2

	if math.Abs(got-want)/want > 1e-12 {
		t.Errorf("r band = %.6e, want %.6e W m^-2 sr^-1 nm^-1", got, want)
	}

	const (
		vZeroFlux      = 3.63e-11
		arcsec2PerSter = 4.254517e10
	)

	mag := -2.5 * math.Log10(got/(vZeroFlux*arcsec2PerSter))
	if math.Abs(mag-27.70) > 0.05 {
		t.Errorf("r band is %.2f mag arcsec^-2, want 27.70 within 0.05", mag)
	}
}

// Interpolation must reproduce the table at its own points, and stay between
// neighbours in between.
func TestExtragalacticInterpolation(t *testing.T) {
	t.Parallel()

	e := NewExtragalacticBackground()

	for i, p := range koushanIGL {
		got, _ := e.at(p.PivotNM)

		want := p.NuInu * 1e-9 / p.PivotNM
		if math.Abs(got-want)/want > 1e-12 {
			t.Errorf("band %s does not reproduce its own row: %.6e vs %.6e", p.Band, got, want)
		}

		if i == 0 {
			continue
		}

		// Midway between two pivots the value must lie between them.
		prev := koushanIGL[i-1]
		mid, _ := e.at(0.5 * (prev.PivotNM + p.PivotNM))

		lo, hi := e.spectral[i-1], e.spectral[i]
		if lo > hi {
			lo, hi = hi, lo
		}

		if mid < lo || mid > hi {
			t.Errorf("between %s and %s the interpolant left the bracket: %.4e not in [%.4e, %.4e]",
				prev.Band, p.Band, mid, lo, hi)
		}
	}
}

// Outside the measured range the endpoint is held and the caller is told.
//
// Nine points do not constrain a slope past their ends. Extrapolating the blue
// end linearly reaches zero near 240 nm and goes negative below it, and a
// component that emits negative light is worse than one that is slightly wrong.
func TestExtragalacticRefusesToExtrapolate(t *testing.T) {
	t.Parallel()

	e := NewExtragalacticBackground()

	blue, extrapolated := e.at(300)
	if !extrapolated {
		t.Error("300 nm is below the u pivot and must be flagged")
	}

	if want := e.spectral[0]; blue != want {
		t.Errorf("below the table = %.4e, want the endpoint %.4e", blue, want)
	}

	red, extrapolated := e.at(3000)
	if !extrapolated {
		t.Error("3000 nm is above the Ks pivot and must be flagged")
	}

	if want := e.spectral[len(e.spectral)-1]; red != want {
		t.Errorf("above the table = %.4e, want the endpoint %.4e", red, want)
	}

	// Nothing anywhere may be negative or zero.
	for _, lambda := range []float64{200, 300, 357.7, 500, 1000, 2154.9, 5000} {
		if v, _ := e.at(lambda); v <= 0 {
			t.Errorf("%.0f nm gives %.4e, which is not a radiance", lambda, v)
		}
	}
}

// The component is isotropic, attenuated, and absent below the horizon.
func TestExtragalacticAddRadiance(t *testing.T) {
	t.Parallel()

	e := NewExtragalacticBackground()
	scene := eblScene(t)
	grid := DefaultOpticalGrid()

	eval := func(alt, az float64) (SpectralRadiance, Flag) {
		dst := NewSpectralRadiance(grid)

		flags, err := e.AddRadiance(context.Background(), dst, grid,
			coord.NewAltAz(angle.Deg(alt), angle.Deg(az)), scene)
		if err != nil {
			t.Fatalf("AddRadiance(%v): %v", alt, err)
		}

		return dst, flags
	}

	below, _ := eval(-10, 0)
	for i := range below {
		if below[i] != 0 {
			t.Fatalf("radiance below the horizon at index %d: %v", i, below[i])
		}
	}

	// Isotropy: two directions at the same altitude must agree exactly, since
	// only the airmass varies and that depends on altitude alone.
	north, _ := eval(60, 0)
	south, _ := eval(60, 180)

	for i := range north {
		if north[i] != south[i] {
			t.Fatalf("azimuth changed the result at index %d: %v vs %v", i, north[i], south[i])
		}
	}

	// Attenuation: the horizon is dimmer than the zenith, and both are below
	// the unattenuated table value.
	zenith, _ := eval(89, 0)
	low, _ := eval(15, 0)

	mid := grid.Len() / 2

	if !(low[mid] < zenith[mid]) {
		t.Errorf("a longer path did not attenuate more: %v at 15 deg vs %v at 89", low[mid], zenith[mid])
	}

	unattenuated, _ := e.at(float64(grid.At(mid)))
	if zenith[mid] >= unattenuated {
		t.Errorf("zenith %v is not below the extra-atmospheric %v", zenith[mid], unattenuated)
	}

	// The optical grid starts blueward of the u pivot, so evaluating over it
	// must report the extrapolation rather than hide it.
	if _, flags := eval(60, 0); grid.At(0) < unit.WavelengthNM(koushanIGL[0].PivotNM) &&
		flags&ExtrapolatedModel == 0 {
		t.Errorf("grid starts at %v, below the %v pivot, but ExtrapolatedModel is unset",
			grid.At(0), koushanIGL[0].PivotNM)
	}
}

// The component must say it is a lower limit. A caller adding it to the others
// is entitled to know the sum is a floor rather than a best estimate.
func TestExtragalacticProvenanceRecordsTheLowerLimit(t *testing.T) {
	t.Parallel()

	p := NewExtragalacticBackground().Provenance()

	if p.PrimaryReference == "" || p.Equations == "" {
		t.Error("provenance must name the paper and the table")
	}

	var saysLimit bool

	for _, a := range p.KnownApproximations {
		if len(a) > 0 && (contains(a, "lower limit") || contains(a, "floor")) {
			saysLimit = true
		}
	}

	if !saysLimit {
		t.Errorf("provenance does not record that the IGL is a lower limit: %v", p.KnownApproximations)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}

	return false
}
