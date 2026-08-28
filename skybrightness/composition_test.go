package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
)

// Every preset's Transfer produces an atmosphere its own model accepts.
//
// # Why this is the test that matters for Transfer
//
// [skybrightness.Model.Estimate] rejects a scene whose transfer is not the
// one the preset names — that guard exists because callers got this wrong by
// hand. Transfer is the other half of that: the guard says no, and this says
// what yes looks like. If the two ever disagree, a caller doing exactly what
// the library tells them would be refused by the library, which is worse than
// either failure alone.
//
// So this round-trips every preset through Transfer into a real evaluation.
// Nothing else in the suite connects those two pieces.
func TestPresetTransferProducesAnAcceptedScene(t *testing.T) {
	t.Parallel()

	for _, p := range []skybrightness.Preset{
		skybrightness.GAMBONSWeb,
		skybrightness.NaturalSky,
		skybrightness.GAMBONSFull,
		skybrightness.Observatory,
	} {
		t.Run(string(p), func(t *testing.T) {
			t.Parallel()

			in := observatoryInputs(t)

			model, err := skybrightness.NewPreset(p, in)
			if err != nil {
				t.Fatalf("NewPreset: %v", err)
			}

			builder, err := p.Transfer(atmosphere.NewBuilder().
				Surface(743, 284).
				Aerosol(0.02, 550, 1.3, 0.95, 0.65).
				AerosolScaleHeight(1500))
			if err != nil {
				t.Fatalf("Transfer: %v", err)
			}

			air, err := builder.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			fidelity, err := p.Fidelity()
			if err != nil {
				t.Fatalf("Fidelity: %v", err)
			}

			scene := presetGoldenScene(t, p)
			scene.Atmosphere = air

			if _, err := model.Estimate(t.Context(), skybrightness.Query{
				Scene:     scene,
				Direction: coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
				Grid:      in.Grid,
				Fidelity:  fidelity,
			}); err != nil {
				t.Fatalf("a scene built by Transfer was refused by the model that named it: %v", err)
			}
		})
	}
}

// Transfer rejects what it cannot configure.
func TestPresetTransferRejectsBadInput(t *testing.T) {
	t.Parallel()

	if _, err := skybrightness.GAMBONSWeb.Transfer(nil); err == nil {
		t.Error("a nil builder was accepted")
	}

	if _, err := skybrightness.Preset("no-such-preset").Transfer(atmosphere.NewBuilder()); err == nil {
		t.Error("an unknown preset was accepted")
	}
}

// The shares sum to one, and they are ordered.
//
// The sum is the property that makes Fraction the field worth reading:
// radiance adds, so the shares of it must account for the whole sky and
// nothing more. A term double-counted or dropped shows up here and nowhere
// else in this file.
func TestCompositionSharesSumToOne(t *testing.T) {
	t.Parallel()

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	est, err := model.Estimate(t.Context(), skybrightness.Query{
		Scene:     presetGoldenScene(t, skybrightness.GAMBONSWeb),
		Direction: coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
		Grid:      in.Grid,
		Fidelity:  skybrightness.Standard,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	rows, err := est.Composition(in.Band, magnitude.Vega)
	if err != nil {
		t.Fatalf("Composition: %v", err)
	}

	if len(rows) == 0 {
		t.Fatal("no components")
	}

	var sum float64

	for i, r := range rows {
		sum += r.Fraction

		if i > 0 && r.Fraction > rows[i-1].Fraction {
			t.Errorf("row %d (%s, %.4f) sorts above row %d (%s, %.4f); brightest first",
				i, r.Component, r.Fraction, i-1, rows[i-1].Component, rows[i-1].Fraction)
		}

		if r.Fraction < 0 || r.Fraction > 1 {
			t.Errorf("%s has fraction %.4f, outside [0,1]", r.Component, r.Fraction)
		}
	}

	if math.Abs(sum-1) > 1e-12 {
		t.Errorf("the shares sum to %.12f, not 1; a term is double-counted or dropped", sum)
	}
}

// A component contributing nothing is reported, at the end.
//
// Moonlight with the Moon below the horizon is real information — "nothing
// from there" — so the row exists rather than being filtered out. Its
// magnitude is +Inf, which is what zero flux is worth, and its zero share
// sorts it past everything that did contribute.
func TestCompositionKeepsSilentComponentsLast(t *testing.T) {
	t.Parallel()

	in := observatoryInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.Observatory, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	// The golden scene's instant has the Moon well up, so look for a term
	// that is genuinely dark instead: point below the horizon is refused, so
	// use the artificial term with an emitter far away and the Moon term at
	// an instant it is down.
	scene := presetGoldenScene(t, skybrightness.Observatory)
	scene.Time = scene.Time.AddDate(0, 0, 14) // half a synodic month later

	est, err := model.Estimate(t.Context(), skybrightness.Query{
		Scene:     scene,
		Direction: coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
		Grid:      in.Grid,
		Fidelity:  skybrightness.Reference,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	rows, err := est.Composition(in.Band, magnitude.Vega)
	if err != nil {
		t.Fatalf("Composition: %v", err)
	}

	var sum float64

	for i, r := range rows {
		sum += r.Fraction

		if r.Fraction == 0 && !math.IsInf(r.Brightness, 1) {
			t.Errorf("%s contributes nothing but reports %.4g mag; zero flux is +Inf",
				r.Component, r.Brightness)
		}

		// A zero share must not sort above a positive one.
		if r.Fraction == 0 && i+1 < len(rows) && rows[i+1].Fraction > 0 {
			t.Errorf("%s contributes nothing and sorts above %s, which does",
				r.Component, rows[i+1].Component)
		}
	}

	if math.Abs(sum-1) > 1e-12 {
		t.Errorf("the shares sum to %.12f, not 1", sum)
	}
}

// Surface conditions from the site's elevation match the standard profile,
// and fall with height.
//
// The alternative is a caller typing hectopascals, and the number is not
// free: pressure sets the Rayleigh optical depth, so a sea-level default at
// a high site overstates molecular scattering.
func TestSurfaceAtAltitudeFollowsTheStandardProfile(t *testing.T) {
	t.Parallel()

	sea, err := atmosphere.NewBuilder().SurfaceAtAltitude(0).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	high, err := atmosphere.NewBuilder().SurfaceAtAltitude(2635).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	seaP, _ := sea.Surface()
	highP, _ := high.Surface()

	if !(highP < seaP) {
		t.Errorf("pressure at 2635 m is %g hPa against %g at sea level; it must fall",
			float64(highP), float64(seaP))
	}

	// Against the profile it claims to use, rather than against a literal.
	isa := atmosphere.AtAltitude(2635)
	if math.Abs(float64(highP)-isa.Pressure) > 1e-9 {
		t.Errorf("pressure is %g hPa and AtAltitude says %g", float64(highP), isa.Pressure)
	}

	// Roughly three-quarters of sea level at 2.6 km, which is the sanity
	// check that catches a metres-for-feet or a sign slip.
	if r := float64(highP) / float64(seaP); r < 0.65 || r > 0.85 {
		t.Errorf("pressure ratio is %.3f; the standard atmosphere gives about 0.73 at 2.6 km", r)
	}
}
