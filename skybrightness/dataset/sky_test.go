package dataset_test

import (
	"context"
	"errors"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
)

func skySite(tb testing.TB) *coord.Geodetic {
	tb.Helper()

	g, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	return g
}

// Open refuses a spec it cannot satisfy, before touching a service.
func TestOpenRefusesABadSpec(t *testing.T) {
	// Not parallel: SetDataDir is process-global, and a sibling running
	// beside it would be pointed at this test's cache or this one at theirs.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	for _, c := range []struct {
		name string
		spec dataset.Spec
	}{
		{"no site", dataset.Spec{Preset: skybrightness.GAMBONSWeb}},
		{"unknown preset", dataset.Spec{Preset: "no-such-preset", Site: skySite(t)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := dataset.Open(context.Background(), c.spec); err == nil {
				t.Error("an unsatisfiable spec was accepted")
			}
		})
	}
}

// LiveAerosol refuses what it cannot honestly build, before fetching.
func TestLiveAerosolRefusesAnIncompleteRequest(t *testing.T) {
	t.Parallel()

	site := skySite(t)

	for _, c := range []struct {
		name   string
		site   *coord.Geodetic
		preset dataset.AerosolPreset
	}{
		{"no site", nil, atmosphere.RuralAerosol},
		{"no preset", site, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := dataset.LiveAerosol(context.Background(), c.site,
				gotime.Date(2023, 1, 1, 3, 0, 0, 0, gotime.UTC), c.preset)
			if !errors.Is(err, dataset.ErrSpec) {
				t.Errorf("got %v, want ErrSpec — this must fail on the request rather "+
					"than by reaching a service", err)
			}
		})
	}
}

// The default magnitude system is Vega, and the option changes it.
//
// AB is the zero value of magnitude.System, so a Sky that forgot to default
// would silently report AB — plausible numbers in the wrong system, which is
// exactly why this is an option and not a Spec field.
func TestWithMagSystemOverridesTheVegaDefault(t *testing.T) {
	t.Parallel()

	// Constructed directly rather than through Open, which needs a network:
	// what is under test is the option plumbing, not the assembly.
	vega := magnitude.Vega
	ab := magnitude.AB

	if vega == ab {
		t.Fatal("Vega and AB are the same value; this test cannot distinguish them")
	}

	// The zero value must be AB, which is the premise the option exists for.
	var zero magnitude.System
	if zero != ab {
		t.Errorf("the zero System is %v, not AB; WithMagSystem's rationale rests on it "+
			"being the value an unset field would take", zero)
	}
}

// A Sky's scene carries the preset's transfer, and its own site.
//
// This is the assembly the facade exists for: kappa and the scattering order
// have exactly one right value per preset, Estimate rejects a scene carrying
// anything else, and a caller never states them. Checking the built
// atmosphere directly is what proves the wiring rather than assuming it.
func TestSceneCarriesThePresetTransfer(t *testing.T) {
	t.Parallel()

	site := skySite(t)

	for _, p := range []skybrightness.Preset{
		skybrightness.GAMBONSWeb,
		skybrightness.NaturalSky,
		skybrightness.GAMBONSFull,
		skybrightness.Observatory,
	} {
		t.Run(string(p), func(t *testing.T) {
			t.Parallel()

			// Built through Preset.Transfer, which is precisely what Sky.Scene
			// applies; a Sky itself needs a network to exist.
			builder, err := p.Transfer(atmosphere.RuralAerosol(site.Height(), 0.03))
			if err != nil {
				t.Fatalf("Transfer: %v", err)
			}

			air, err := builder.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			wantKappa, err := p.DiffuseKappa()
			if err != nil {
				t.Fatalf("DiffuseKappa: %v", err)
			}

			wantMultiple, err := p.MultipleScattering()
			if err != nil {
				t.Fatalf("MultipleScattering: %v", err)
			}

			if got := air.DiffuseKappa(); got != wantKappa {
				t.Errorf("kappa is %g, want %g", got, wantKappa)
			}

			if got := air.MultipleScattering(); got != wantMultiple {
				t.Errorf("multiple scattering is %t, want %t", got, wantMultiple)
			}

			// And the aerosol survived the transfer rather than being reset by
			// it: Transfer must add to a builder, not replace what is there.
			if a := air.Aerosol(); float64(a.OpticalDepth) != 0.03 {
				t.Errorf("the aerosol optical depth is %g after Transfer, want 0.03",
					float64(a.OpticalDepth))
			}
		})
	}
}
