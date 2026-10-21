package skybrightness_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
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
//
// # Why this instant and not another
//
// The Moon is 73 degrees up and 3 degrees from full here, with the Sun 70
// degrees down. That matters only for [skybrightness.Observatory], and it
// matters entirely: at the instant this fixture used before, the Moon was
// below the horizon, so its moonlight term evaluated to exactly zero at every
// altitude. Locking a column of zeros would have guarded nothing about the
// largest capability difference between that preset and GAMBONS, while looking
// in every way like coverage.
//
// The three reproductions have no moonlight term and are indifferent to the
// Moon. Their numbers still move with the date, through the zodiacal light's
// dependence on solar elongation, which is why all four tables were regenerated
// together rather than two of them being left alone.
//
// #588 moved the zodiacal rows of GAMBONSFull and Observatory, and the totals
// with them, by up to 3e-5 relative and nothing else: their scattered-light
// integrals sample the sky below 10° of altitude, where refraction now hands
// over to Bennett-NA instead of holding SOFA's clamp, and the zodiacal table
// is the one smooth enough to follow the arcminutes that moved.
//
// #646 moved Observatory's moonlight rows by 0.7 to 1.9 per cent, and its
// totals with them. The Moon is placed where the site sees it, which lowers
// it into more air and on its own dims the zenith by 1.0 per cent, and its
// phase angle is the site's, which brightens every row by about 1.7.
//
// #672 moved every zodiacal row by 2 to 3 parts in 10^12. The Sun's ecliptic
// longitude, which zodiacal light is a function of, now comes from the
// instant read as TT, where it used to read this fixed UTC instant's Julian
// Date as TT, 69 seconds early.
//
// #803 moved Observatory's moonlight rows by 3e-10 to 4e-8 relative, and its
// totals with them by up to 1.3e-9. The Moon's apparent place is now SOFA's,
// the astrometric place deflected and aberrated relativistically, where it
// was the geocentric state at the retarded epoch: milliarcseconds of the
// Moon's direction, which its scattered light follows.
//
// #592 moved every component row by up to 0.2 per cent, Observatory's
// moonlight by 0.81, and the totals by at most 1.3e-4 relative. The Rayleigh
// optical depth is Bodhaine et al.'s (1999) now, 0.7 per cent below the
// power law it replaced at 550 nm and 4 per cent above it at 330 nm.
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
		AerosolScaleHeight(1500).
		DiffuseScattering(kappa).
		MultipleScattering(multiple).
		Build()
	if err != nil {
		tb.Fatalf("atmosphere Build: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       time.GoDate(2026, 4, 2, 5, 0, 0, 0, time.LocationUTC),
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

// goldenComponentsFor is the list a given preset should produce.
//
// The three reproductions carry the five natural components of Eq. 10.
// [skybrightness.Observatory] carries two more, and they have to be locked
// too: they are the largest capability difference between it and GAMBONS, so
// locking the five it shares and omitting the two it does not would leave its
// most distinctive half unguarded.
func goldenComponentsFor(p skybrightness.Preset) []skybrightness.ComponentID {
	if p != skybrightness.Observatory {
		return goldenComponents
	}

	out := make([]skybrightness.ComponentID, 0, len(goldenComponents)+2)
	out = append(out, goldenComponents...)

	return append(out, skybrightness.Moonlight, skybrightness.Artificial)
}

// goldenInputs supplies what a given preset needs.
//
// [skybrightness.Observatory] refuses without a solar spectrum and a ground
// emitter, since it is the only preset with terms that consume them.
func goldenInputs(tb testing.TB, p skybrightness.Preset) skybrightness.PresetInputs {
	tb.Helper()

	if p == skybrightness.Observatory {
		return observatoryInputs(tb)
	}

	return presetInputs(tb)
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
	{90, skybrightness.Starlight, 9.7296421351210988e-10},
	{90, skybrightness.DiffuseGalactic, 4.2028432317607663e-11},
	{90, skybrightness.Extragalactic, 1.2145000987858515e-11},
	{90, skybrightness.Zodiacal, 1.651377714431181e-09},
	{90, skybrightness.AirglowContinuum, 2.4324105337802741e-09},
	{90, totalSurfaceBrightness, 21.220646241569941},

	{60, skybrightness.Starlight, 9.6886462111064962e-10},
	{60, skybrightness.DiffuseGalactic, 4.1851345185951912e-11},
	{60, skybrightness.Extragalactic, 1.2093827930233068e-11},
	{60, skybrightness.Zodiacal, 1.694790709370695e-09},
	{60, skybrightness.AirglowContinuum, 2.7844924308704943e-09},
	{60, totalSurfaceBrightness, 21.140674906005771},

	{30, skybrightness.Starlight, 9.4683701801240366e-10},
	{30, skybrightness.DiffuseGalactic, 4.0899834726392516e-11},
	{30, skybrightness.Extragalactic, 1.181886893618881e-11},
	{30, skybrightness.Zodiacal, 1.0029498195693226e-09},
	{30, skybrightness.AirglowContinuum, 4.5550517100434872e-09},
	{30, totalSurfaceBrightness, 20.950672880606035},

	{10, skybrightness.Starlight, 8.5816689211719126e-10},
	{10, skybrightness.DiffuseGalactic, 3.7069615348304054e-11},
	{10, skybrightness.Extragalactic, 1.0712046350491017e-11},
	{10, skybrightness.Zodiacal, 7.1692977781471784e-10},
	{10, skybrightness.AirglowContinuum, 9.0595212308208101e-09},
	{10, totalSurfaceBrightness, 20.423232580029818},
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
	{90, skybrightness.Starlight, 9.9206806205412118e-10},
	{90, skybrightness.DiffuseGalactic, 4.2853647463553752e-11},
	{90, skybrightness.Extragalactic, 1.2383464290200449e-11},
	{90, skybrightness.Zodiacal, 1.6722492912414451e-09},
	{90, skybrightness.AirglowContinuum, 2.578856830452819e-09},
	{90, totalSurfaceBrightness, 21.181393965376238},

	{60, skybrightness.Starlight, 9.8985605253850885e-10},
	{60, skybrightness.DiffuseGalactic, 4.275809688633654e-11},
	{60, skybrightness.Extragalactic, 1.2355852937820609e-11},
	{60, skybrightness.Zodiacal, 1.7166923937106659e-09},
	{60, skybrightness.AirglowContinuum, 2.9517915060079003e-09},
	{60, totalSurfaceBrightness, 21.099682321740264},

	{30, skybrightness.Starlight, 9.7669069002461094e-10},
	{30, skybrightness.DiffuseGalactic, 4.2189402231725533e-11},
	{30, skybrightness.Extragalactic, 1.2191516635913224e-11},
	{30, skybrightness.Zodiacal, 1.058197262863035e-09},
	{30, skybrightness.AirglowContinuum, 4.7759022079010848e-09},
	{30, totalSurfaceBrightness, 20.901196154634341},

	{10, skybrightness.Starlight, 9.1122171100073164e-10},
	{10, skybrightness.DiffuseGalactic, 3.9361386035861891e-11},
	{10, skybrightness.Extragalactic, 1.1374301774485845e-11},
	{10, skybrightness.Zodiacal, 8.4367747268822607e-10},
	{10, skybrightness.AirglowContinuum, 8.8770292552346447e-09},
	{10, totalSurfaceBrightness, 20.425037211081364},
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

// The Duriscoe transfer, locked separately from the two GAMBONS presets.
//
// It shares their components and differs only in kappa, which is exactly the
// case a shared lock would hide: the components could change while this table
// and one of theirs moved together, and a per-preset lock is what makes the
// difference attributable.
func TestNaturalSkyPresetGolden(t *testing.T) {
	t.Parallel()

	checkPresetGolden(t, skybrightness.NaturalSky, naturalSkyGolden)
}

// This module's own model, locked.
//
// # Why this one matters most
//
// The other three reproduce a published model, so each has an external
// reference: a wrong change shows up as a widening gap against GAMBONS' own
// export or against Table 2. [skybrightness.Observatory] is not reproducing
// anybody. It has no published counterpart by construction, which means no
// external comparison can ever be written for it and a lock like this one is
// the only regression protection it can have.
//
// It went without for a while. Its components were checked for presence and
// its accessors for value, and its numbers were asserted nowhere - the preset
// with the least external validation had the least internal validation too.
func TestObservatoryPresetGolden(t *testing.T) {
	t.Parallel()

	checkPresetGolden(t, skybrightness.Observatory, observatoryGolden)
}

// naturalSkyGolden is the locked output of [skybrightness.NaturalSky].
//
// Same caveat as every table here: evidence that the preset does not change
// silently, not evidence that it is right.
var naturalSkyGolden = []goldenRow{
	{90, skybrightness.Starlight, 9.5972166820414981e-10},
	{90, skybrightness.DiffuseGalactic, 4.1456403653593991e-11},
	{90, skybrightness.Extragalactic, 1.1979701253692152e-11},
	{90, skybrightness.Zodiacal, 1.6289016110964326e-09},
	{90, skybrightness.AirglowContinuum, 2.3993041705103741e-09},
	{90, totalSurfaceBrightness, 21.235875472280906},

	{60, skybrightness.Starlight, 9.5366236991056069e-10},
	{60, skybrightness.DiffuseGalactic, 4.1194664522094954e-11},
	{60, skybrightness.Extragalactic, 1.19040662172341e-11},
	{60, skybrightness.Zodiacal, 1.6681981044452565e-09},
	{60, skybrightness.AirglowContinuum, 2.7408015451920453e-09},
	{60, totalSurfaceBrightness, 21.158249592484463},

	{30, skybrightness.Starlight, 9.2132497378589233e-10},
	{30, skybrightness.DiffuseGalactic, 3.9797809380375621e-11},
	{30, skybrightness.Extragalactic, 1.1500415494602478e-11},
	{30, skybrightness.Zodiacal, 9.7592584430530411e-10},
	{30, skybrightness.AirglowContinuum, 4.4323181471705229e-09},
	{30, totalSurfaceBrightness, 20.981015442565067},

	{10, skybrightness.Starlight, 7.9498261842879024e-10},
	{10, skybrightness.DiffuseGalactic, 3.4340289918477125e-11},
	{10, skybrightness.Extragalactic, 9.9233502651614595e-12},
	{10, skybrightness.Zodiacal, 6.6414437241990837e-10},
	{10, skybrightness.AirglowContinuum, 8.3924956508408589e-09},
	{10, totalSurfaceBrightness, 20.50780894778033},
}

// observatoryGolden is the locked output of [skybrightness.Observatory],
// including its moonlight and artificial-skyglow terms.
//
// The scene is the fixed instant presetGoldenScene defines, so the Moon's
// position - and with it the whole moonlight term - is a constant of the
// fixture rather than of the day this runs.
var observatoryGolden = []goldenRow{
	{90, skybrightness.Starlight, 1.0066121844124125e-09},
	{90, skybrightness.DiffuseGalactic, 4.3481899411226068e-11},
	{90, skybrightness.Extragalactic, 1.2565010926711747e-11},
	{90, skybrightness.Zodiacal, 1.6932342549546409e-09},
	{90, skybrightness.AirglowContinuum, 2.6468258597986148e-09},
	{90, skybrightness.Moonlight, 1.0244784683399866e-07},
	{90, skybrightness.Artificial, 8.4719605297296167e-08},
	{90, totalSurfaceBrightness, 17.275891330770328},

	{60, skybrightness.Starlight, 1.0062414465784372e-09},
	{60, skybrightness.DiffuseGalactic, 4.3465884916811611e-11},
	{60, skybrightness.Extragalactic, 1.2560383201151721e-11},
	{60, skybrightness.Zodiacal, 1.7406086097754482e-09},
	{60, skybrightness.AirglowContinuum, 3.0331446089703345e-09},
	{60, skybrightness.Moonlight, 8.9325085795414407e-08},
	{60, skybrightness.Artificial, 1.1758632803258909e-07},
	{60, totalSurfaceBrightness, 17.166795544593661},

	{30, skybrightness.Starlight, 1.0023751568576856e-09},
	{30, skybrightness.DiffuseGalactic, 4.3298875592529957e-11},
	{30, skybrightness.Extragalactic, 1.2512122338289786e-11},
	{30, skybrightness.Zodiacal, 1.0929706713183126e-09},
	{30, skybrightness.AirglowContinuum, 4.924201610094471e-09},
	{30, skybrightness.Moonlight, 7.4884756193961823e-08},
	{30, skybrightness.Artificial, 2.6080072391243097e-07},
	{30, totalSurfaceBrightness, 16.650234412986332},

	{10, skybrightness.Starlight, 9.671998615786218e-10},
	{10, skybrightness.DiffuseGalactic, 4.1779433770973605e-11},
	{10, skybrightness.Extragalactic, 1.2073047611820291e-11},
	{10, skybrightness.Zodiacal, 9.1684283378512596e-10},
	{10, skybrightness.AirglowContinuum, 9.2301364630354105e-09},
	{10, skybrightness.Moonlight, 1.311328641596625e-07},
	{10, skybrightness.Artificial, 7.8275133537989355e-07},
	{10, totalSurfaceBrightness, 15.576595000913994},
}

// checkPresetGolden compares one preset against its own locked table.
//
// The fidelity comes from the preset rather than from the caller. Asking
// GAMBONSFull at Standard would evaluate a sky with no scattering treatment at
// all and lock those numbers instead, which is the one mistake here that
// produces a plausible table rather than an error.
func checkPresetGolden(t *testing.T, p skybrightness.Preset, table []goldenRow) {
	t.Helper()

	in := goldenInputs(t, p)

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

		for _, id := range goldenComponentsFor(p) {
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

	// Moonlight and Artificial are [skybrightness.Observatory]'s own two
	// terms; the other two are registered by no preset today. All four are
	// named so that a preset gaining one generates a pasteable line rather
	// than a quoted string literal that would compile but read as an
	// accident - which is exactly how Observatory's two came to be locked.
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
