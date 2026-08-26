package skybrightness_test

import (
	"fmt"
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
)

// goldenTol is how far a preset's output may move before this test calls it a
// change.
//
// Relative, not absolute: the radiances here span several orders of magnitude,
// so one absolute bound would be vacuous for the bright components and
// impossible for the faint ones.
//
// 1e-12 is loose enough to survive the one thing that legitimately differs
// between platforms — Go fuses a multiply-add into an FMA on arm64 and ppc64
// but not on amd64, which perturbs a spectral sum in its last few bits, on the
// order of 1e-15 relative after several hundred grid points — and tight enough
// that no change to a coefficient, a transfer term or a component can slip
// through. It is not a physical tolerance, and it must never be relaxed to make
// a deliberate change pass: a deliberate change updates the table.
const goldenTol = 1e-12

// presetGoldenScene is the fixed scene the locked numbers below belong to.
//
// Every input is written here rather than fetched, so the numbers reproduce on
// any machine and in any year. The site is Cerro Paranal, the instant is a
// fixed UTC one, and the atmosphere carries the preset's own transfer factor.
func presetGoldenScene(tb testing.TB, p skybrightness.Preset) *skybrightness.Scene {
	tb.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	kappa, err := p.DiffuseKappa()
	if err != nil {
		tb.Fatalf("DiffuseKappa: %v", err)
	}

	multiple, err := p.MultipleScattering()
	if err != nil {
		tb.Fatalf("MultipleScattering: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0.02, 550, 1.3, 0.95, 0.65).
		BoundaryLayer(1500).
		DiffuseScattering(kappa).
		MultipleScattering(multiple).
		Build()
	if err != nil {
		tb.Fatalf("atmosphere Build: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       gotime.Date(2026, 3, 20, 5, 0, 0, 0, gotime.UTC),
		Atmosphere: atm,
		Ephemeris:  eph.Default(),
	}
}

// goldenAltitudes are the viewing directions the table covers: the zenith,
// where the airmass is one and the geometry is least forgiving of a transfer
// error; two mid-altitudes; and ten degrees, where the airmass is steep enough
// that an airmass or scale-height change shows immediately.
var goldenAltitudes = []float64{90, 60, 30, 10}

// goldenComponents are locked individually so a drift is attributable.
//
// A single total would say "something moved". These say which component moved,
// which is the difference between a failure that is a lead and one that is a
// puzzle.
var goldenComponents = []skybrightness.ComponentID{
	skybrightness.Starlight,
	skybrightness.DiffuseGalactic,
	skybrightness.Extragalactic,
	skybrightness.Zodiacal,
	skybrightness.AirglowContinuum,
}

// totalSurfaceBrightness is not a component; it labels the whole-band row.
const totalSurfaceBrightness skybrightness.ComponentID = "total-surface-brightness"

// goldenRow is one locked evaluation: a component's spectral radiance at
// 550 nm, in W m-2 sr-1 nm-1, or the band-integrated surface brightness in
// mag/arcsec2 when the component is [totalSurfaceBrightness].
type goldenRow struct {
	altDeg float64
	id     skybrightness.ComponentID
	value  float64
}

// gambonsWebGolden is the locked output of [skybrightness.GAMBONSWeb] over
// presetInputs and presetGoldenScene.
//
// # What this test is and is not
//
// It is a regression lock. These values were produced by this code, so they are
// evidence that the preset does not change silently — nothing more. They are
// NOT evidence that the preset is right: correctness against GAMBONS is
// established by the validation-tagged comparisons against the published
// all-sky export, which need a network and a star map and cannot run here.
//
// The two are complementary and neither substitutes for the other. This one
// runs on every commit and catches an accidental change in seconds; that one
// runs rarely and catches a wrong change. A number moving here is not
// automatically a bug, but it must be explained, and the explanation belongs in
// the commit that moves it.
var gambonsWebGolden = []goldenRow{
	{90, skybrightness.Starlight, 9.7273884516322964e-10},
	{90, skybrightness.DiffuseGalactic, 4.2018697243834238e-11},
	{90, skybrightness.Extragalactic, 1.214218783318975e-11},
	{90, skybrightness.Zodiacal, 1.5220545545442494e-09},
	{90, skybrightness.AirglowContinuum, 2.4318471129080736e-09},
	{90, totalSurfaceBrightness, 21.248628500932224},

	{60, skybrightness.Starlight, 9.6860563353807089e-10},
	{60, skybrightness.DiffuseGalactic, 4.1840157886856984e-11},
	{60, skybrightness.Extragalactic, 1.2090595124461664e-11},
	{60, skybrightness.Zodiacal, 1.826480625985605e-09},
	{60, skybrightness.AirglowContinuum, 2.7837481071333883e-09},
	{60, totalSurfaceBrightness, 21.115182920107912},

	{30, skybrightness.Starlight, 9.4639993687408995e-10},
	{30, skybrightness.DiffuseGalactic, 4.0880954448183097e-11},
	{30, skybrightness.Extragalactic, 1.1813413082023905e-11},
	{30, skybrightness.Zodiacal, 1.0675872220787639e-09},
	{30, skybrightness.AirglowContinuum, 4.5529489963254674e-09},
	{30, totalSurfaceBrightness, 20.940422609961743},

	{10, skybrightness.Starlight, 8.5705815421231311e-10},
	{10, skybrightness.DiffuseGalactic, 3.7021721997915625e-11},
	{10, skybrightness.Extragalactic, 1.0698206557862453e-11},
	{10, skybrightness.Zodiacal, 7.4487976873245172e-10},
	{10, skybrightness.AirglowContinuum, 9.0478164742275109e-09},
	{10, totalSurfaceBrightness, 20.421634636523333},
}

// gambonsFullGolden is the locked output of [skybrightness.GAMBONSFull].
//
// Locked separately from [skybrightness.GAMBONSWeb] and not derived from it,
// because they are different models rather than two settings of one. The web
// preset puts the scattered light into an effective optical depth; this one
// puts it into the Eq. 11 integral and carries the true extinction in the
// direct term. Nothing about either table predicts the other, and a single
// lock over both would hide a change that moved one and not the other.
var gambonsFullGolden = []goldenRow{
	{90, skybrightness.Starlight, 9.9185382323796635e-10},
	{90, skybrightness.DiffuseGalactic, 4.2844393144166162e-11},
	{90, skybrightness.Extragalactic, 1.238079005964017e-11},
	{90, skybrightness.Zodiacal, 1.5462146488353121e-09},
	{90, skybrightness.AirglowContinuum, 2.5788686481672761e-09},
	{90, totalSurfaceBrightness, 21.207547398738487},

	{60, skybrightness.Starlight, 9.8960429863641313e-10},
	{60, skybrightness.DiffuseGalactic, 4.2747222054880267e-11},
	{60, skybrightness.Extragalactic, 1.2352710426156573e-11},
	{60, skybrightness.Zodiacal, 1.8455650514032244e-09},
	{60, skybrightness.AirglowContinuum, 2.9516262945475295e-09},
	{60, totalSurfaceBrightness, 21.075559243200498},

	{30, skybrightness.Starlight, 9.7624049428941108e-10},
	{30, skybrightness.DiffuseGalactic, 4.2169955451747658e-11},
	{30, skybrightness.Extragalactic, 1.2185897079127065e-11},
	{30, skybrightness.Zodiacal, 1.1225177167672987e-09},
	{30, skybrightness.AirglowContinuum, 4.7739343029753323e-09},
	{30, totalSurfaceBrightness, 20.891442859700454},

	{10, skybrightness.Starlight, 9.1001151633863401e-10},
	{10, skybrightness.DiffuseGalactic, 3.9309110131218376e-11},
	{10, skybrightness.Extragalactic, 1.135919555047209e-11},
	{10, skybrightness.Zodiacal, 8.7347407170088064e-10},
	{10, skybrightness.AirglowContinuum, 8.8603798273970605e-09},
	{10, totalSurfaceBrightness, 20.423740990972878},
}

// The GAMBONS web preset produces exactly the numbers it produced when they
// were recorded.
func TestGAMBONSWebPresetGolden(t *testing.T) {
	t.Parallel()

	checkPresetGolden(t, skybrightness.GAMBONSWeb, gambonsWebGolden)
}

// The full GAMBONS preset likewise, and separately.
//
// This runs the hemispheric integral at every altitude, so it is the expensive
// test in this file by three orders of magnitude — about a third of a second
// against a hundredth. That is the cost of the model and not of the test.
func TestGAMBONSFullPresetGolden(t *testing.T) {
	t.Parallel()

	checkPresetGolden(t, skybrightness.GAMBONSFull, gambonsFullGolden)
}

// checkPresetGolden compares one preset against its own locked table.
//
// The fidelity comes from the preset rather than from the caller. Asking
// GAMBONSFull at Standard would evaluate a sky with no scattering treatment at
// all and lock those numbers instead, which is the one mistake here that
// produces a plausible table rather than an error.
func checkPresetGolden(t *testing.T, p skybrightness.Preset, table []goldenRow) {
	t.Helper()

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(p, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	fidelity, err := p.Fidelity()
	if err != nil {
		t.Fatalf("Fidelity: %v", err)
	}

	scene := presetGoldenScene(t, p)

	// 550 nm on the 400 nm, 1 nm grid presetInputs defines.
	const idx550 = 150

	if got := float64(in.Grid.At(idx550)); got != 550 {
		t.Fatalf("index %d of the grid is %g nm, want 550 — the table is indexed by wavelength",
			idx550, got)
	}

	// An empty table prints itself rather than passing vacuously, so
	// regenerating after a deliberate change is one run and a paste.
	generate := len(table) == 0

	want := make(map[string]float64, len(table))
	for _, row := range table {
		want[goldenKey(row.altDeg, row.id)] = row.value
	}

	check := func(altDeg float64, id skybrightness.ComponentID, got float64) {
		t.Helper()

		if generate {
			t.Logf("	{%g, %s, %.17g},", altDeg, goldenGoName(id), got)

			return
		}

		w, ok := want[goldenKey(altDeg, id)]
		if !ok {
			t.Errorf("%s: alt %g, %s: not in the table", p, altDeg, id)

			return
		}

		if w == 0 {
			if got != 0 {
				t.Errorf("%s: alt %g, %s: got %.17g, want exactly 0", p, altDeg, id, got)
			}

			return
		}

		if rel := math.Abs(got-w) / math.Abs(w); rel > goldenTol {
			t.Errorf("%s: alt %g, %s: got %.17g, want %.17g — relative %.3g, over the %.0e lock",
				p, altDeg, id, got, w, rel, goldenTol)
		}
	}

	for _, altDeg := range goldenAltitudes {
		est, err := model.Estimate(t.Context(), skybrightness.Query{
			Scene:     scene,
			Direction: coord.NewAltAz(angle.Deg(altDeg), angle.Deg(45)),
			Grid:      in.Grid,
			Fidelity:  fidelity,
		})
		if err != nil {
			t.Fatalf("%s: alt %g: Estimate: %v", p, altDeg, err)
		}

		for _, id := range goldenComponents {
			spec, ok := est.Component(id)
			if !ok {
				t.Fatalf("%s: alt %g: the estimate carries no %s component", p, altDeg, id)
			}

			check(altDeg, id, spec[idx550])
		}

		// The band-integrated surface brightness, which is what a caller
		// actually reads off, and where a compensating pair of component
		// changes would still show.
		sb, err := est.SurfaceBrightness(in.Band, magnitude.Vega)
		if err != nil {
			t.Fatalf("%s: alt %g: SurfaceBrightness: %v", p, altDeg, err)
		}

		check(altDeg, totalSurfaceBrightness, sb)
	}

	if generate {
		t.Fatalf("%s: the golden table is empty; the logged lines above are its contents", p)
	}
}

// The two presets differ numerically, which is what makes locking them
// separately worth doing rather than locking one and assuming the other.
func TestPresetsDifferNumerically(t *testing.T) {
	t.Parallel()

	in := presetInputs(t)

	at := func(p skybrightness.Preset) float64 {
		t.Helper()

		model, err := skybrightness.NewPreset(p, in)
		if err != nil {
			t.Fatalf("%s: NewPreset: %v", p, err)
		}

		est, err := model.Estimate(t.Context(), skybrightness.Query{
			Scene:     presetGoldenScene(t, p),
			Direction: coord.NewAltAz(angle.Deg(30), angle.Deg(45)),
			Grid:      in.Grid,
		})
		if err != nil {
			t.Fatalf("%s: Estimate: %v", p, err)
		}

		sb, err := est.SurfaceBrightness(in.Band, magnitude.Vega)
		if err != nil {
			t.Fatalf("%s: SurfaceBrightness: %v", p, err)
		}

		return sb
	}

	web, natural := at(skybrightness.GAMBONSWeb), at(skybrightness.NaturalSky)

	// A higher kappa is a larger effective optical depth, so the diffuse
	// components are attenuated harder and the sky comes out fainter, which is
	// a larger magnitude. Locking the sign as well as the difference means a
	// transfer term that changed direction would fail here rather than pass by
	// still being unequal.
	if natural <= web {
		t.Errorf("natural-sky is %.6f and gambons-web is %.6f mag/arcsec2; the higher kappa "+
			"of natural-sky attenuates the diffuse components more, so it must be the fainter",
			natural, web)
	}

	if math.Abs(natural-web) < 1e-6 {
		t.Errorf("the presets agree to %.3g mag; nothing distinguishes them numerically",
			math.Abs(natural-web))
	}
}

// goldenKey identifies one row of the table.
func goldenKey(altDeg float64, id skybrightness.ComponentID) string {
	return fmt.Sprintf("%s@%g", id, altDeg)
}

// goldenGoName maps a component back to the Go expression naming it, so the
// generator prints a line that can be pasted into the table unedited.
func goldenGoName(id skybrightness.ComponentID) string {
	switch id {
	case skybrightness.Starlight:
		return "skybrightness.Starlight"
	case skybrightness.DiffuseGalactic:
		return "skybrightness.DiffuseGalactic"
	case skybrightness.Extragalactic:
		return "skybrightness.Extragalactic"
	case skybrightness.Zodiacal:
		return "skybrightness.Zodiacal"
	case skybrightness.AirglowContinuum:
		return "skybrightness.AirglowContinuum"

	// Not registered by either preset today. Named anyway, so that a preset
	// which later gains one of them generates a pasteable line rather than a
	// quoted string literal that would compile but read as an accident.
	case skybrightness.AirglowLines:
		return "skybrightness.AirglowLines"
	case skybrightness.Moonlight:
		return "skybrightness.Moonlight"
	case skybrightness.Twilight:
		return "skybrightness.Twilight"
	case skybrightness.Artificial:
		return "skybrightness.Artificial"

	case totalSurfaceBrightness:
		return "totalSurfaceBrightness"
	default:
		return fmt.Sprintf("skybrightness.ComponentID(%q)", string(id))
	}
}
