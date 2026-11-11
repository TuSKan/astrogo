package dataset_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
	"github.com/TuSKan/astrogo/unit"
)

// Assembling a model's reference data is one call.
//
// This is the shape the package exists for: a caller reaches one package
// rather than five, and the result goes straight into NewPreset.
//
// deterministic output to assert. Kept as an example rather than moved into
// prose because this flow is the first thing a reader needs and pkg.go.dev
// renders it where they will look; TestEndpointsCoverWhatEachPresetFetches
// covers the half of it that can be checked offline.
//
//nolint:testableexamples // Every step reaches a service, so there is no
func Example() {
	ctx := context.Background()

	// Consent first, and only for what this preset actually fetches.
	ids, size := dataset.Endpoints(skybrightness.GAMBONSWeb)
	remote.EnableDownloads(size, ids...)

	in, err := dataset.Inputs(ctx, dataset.Spec{Preset: skybrightness.GAMBONSWeb})
	if err != nil {
		panic(err)
	}

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, in)
	if err != nil {
		panic(err)
	}

	fmt.Println(model.Version())
}

// Endpoints names what a preset will download, and the cap consent needs.
//
// A caller has to be able to see the cost before agreeing to it, which is the
// whole reason this is separate from Inputs.
func TestEndpointsCoverWhatEachPresetFetches(t *testing.T) {
	t.Parallel()

	for _, p := range []skybrightness.Preset{
		skybrightness.GAMBONSWeb,
		skybrightness.NaturalSky,
		skybrightness.GAMBONSFull,
		skybrightness.Observatory,
	} {
		ids, size := dataset.Endpoints(p)

		if len(ids) < 2 {
			t.Errorf("%s: %d endpoints, want at least the star and dust maps", p, len(ids))
		}

		if size <= 0 {
			t.Errorf("%s: consent cap is %d; a zero cap denies every download", p, size)
		}

		// The cap has to clear the largest registered object, or consent is
		// granted for less than the fetch actually moves and every run fails
		// on a size check rather than on anything real.
		for _, id := range ids {
			e, ok := remote.Lookup(id)
			if !ok {
				t.Errorf("%s: endpoint %s is not registered", p, id)

				continue
			}

			if e.ApproxSize > size {
				t.Errorf("%s: %s needs %d bytes and the cap is %d", p, id, e.ApproxSize, size)
			}
		}

		// Only the module's own model has a moonlight term to read a solar
		// spectrum for; charging the others for it would be asking callers to
		// download something no component of theirs opens.
		var hasSolar bool

		for _, id := range ids {
			if id == remote.CALSPEC {
				hasSolar = true
			}
		}

		if want := p == skybrightness.Observatory; hasSolar != want {
			t.Errorf("%s: solar spectrum included = %t, want %t", p, hasSolar, want)
		}
	}
}

// Inputs refuses what it cannot honestly supply, before it fetches anything.
//
// One of these is the package declining to guess: a ground-emitter inventory
// has no default because satellite radiance alone cannot determine a source
// spectrum or an upward emission function, so any default would be reporting
// somebody else's city.
//
// # Why offline, with an empty cache
//
// So that any fetch fails with remote.ErrOffline. A case that comes back
// ErrSpec was therefore refused before the first fetch, which is the
// property: until #1067 most of these were found after one to four fetches,
// two of them large downloads, and this test accepted the emitter case
// failing on the offline passband fetch instead, since "what matters is that
// it fails". Two were not refused at all: a set but invalid grid was replaced
// with the default, and a star shape of the wrong length was returned.
func TestInputsRefusesAnIncompleteSpec(t *testing.T) {
	// Not parallel: offline mode and the data directory are process-global.
	// Captured first, so the tests after this one do not inherit them (#971).
	t.Cleanup(remote.Capture().Restore)
	remote.SetOffline(true)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	web := skybrightness.GAMBONSWeb
	nan := math.NaN()

	cases := []struct {
		name string
		spec dataset.Spec
		want error
	}{
		{"unknown preset", dataset.Spec{Preset: "no-such-preset"}, skybrightness.ErrPreset},
		{"observatory without emitters", dataset.Spec{Preset: skybrightness.Observatory}, dataset.ErrSpec},
		{"grid of one sample", dataset.Spec{Preset: web, Grid: unit.SpectralGrid{StartNM: 400, StepNM: 1, N: 1}}, dataset.ErrSpec},
		{"grid with a negative step", dataset.Spec{Preset: web, Grid: unit.SpectralGrid{StartNM: 400, StepNM: -1, N: 100}}, dataset.ErrSpec},
		{"star shape off the grid", dataset.Spec{Preset: web, StarShape: make(skybrightness.SpectralRadiance, 3)}, dataset.ErrSpec},
		{"negative star temperature", dataset.Spec{Preset: web, StarTemperatureK: -1}, dataset.ErrSpec},
		{"NaN star temperature", dataset.Spec{Preset: web, StarTemperatureK: nan}, dataset.ErrSpec},
		{"unknown airglow observatory", dataset.Spec{Preset: web, Observatory: "nowhere"}, dataset.ErrSpec},
		{"negative solar flux", dataset.Spec{Preset: web, SolarFluxSFU: -1}, dataset.ErrSpec},
		{"NaN airglow scale", dataset.Spec{Preset: web, AirglowScale: nan}, dataset.ErrSpec},
		{"negative airglow scale", dataset.Spec{Preset: web, AirglowScale: -2}, dataset.ErrSpec},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := dataset.Inputs(context.Background(), c.spec)
			if errors.Is(err, remote.ErrOffline) {
				t.Fatalf("Inputs fetched before refusing the spec: %v", err)
			}

			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}

	// A spec with every field set, and set right, passes those checks and
	// reaches the first fetch, so the checks above refuse only what is wrong.
	good := dataset.Spec{
		Preset:           web,
		Grid:             unit.SpectralGrid{StartNM: 400, StepNM: 1, N: 301},
		StarTemperatureK: 5000,
		Observatory:      "armazones",
		SolarFluxSFU:     100,
		AirglowScale:     1.5,
	}

	if _, err := dataset.Inputs(context.Background(), good); !errors.Is(err, remote.ErrOffline) {
		t.Errorf("a valid spec returned %v; it should pass every check and fail on the first "+
			"fetch, offline", err)
	}
}

// The default band and the default map column name the same band.
//
// They come from different authorities — SVO calls Johnson V
// "Generic/Bessell.V" and the published star map calls it "V" — so nothing but
// this pairing keeps them describing the same thing. Deriving one from the
// other would work for the Bessell curves and silently read the wrong column
// for any other filter.
func TestDefaultBandAndMapColumnAgree(t *testing.T) {
	t.Parallel()

	if dataset.DefaultMapBand != "V" {
		t.Errorf("default map column is %q, want V", dataset.DefaultMapBand)
	}

	if dataset.DefaultBandID != "Generic/Bessell.V" {
		t.Errorf("default band is %q, which is not the Johnson V the map column names",
			dataset.DefaultBandID)
	}
}
