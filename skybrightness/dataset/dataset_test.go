package dataset_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
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

	site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		panic(err)
	}

	// Consent first, and only for what this preset actually fetches.
	ids, size := dataset.Endpoints(skybrightness.GAMBONSWeb)
	remote.EnableDownloads(size, ids...)

	in, err := dataset.Inputs(ctx, dataset.Spec{
		Preset: skybrightness.GAMBONSWeb,
		Site:   site,
	})
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

// Inputs refuses what it cannot honestly supply.
//
// Two of these are the package declining to guess. A site has no default
// because a sky brightness without a place is not a number; a ground-emitter
// inventory has none because satellite radiance alone cannot determine a
// source spectrum or an upward emission function, so any default would be
// reporting somebody else's city.
func TestInputsRefusesAnIncompleteSpec(t *testing.T) {
	t.Parallel()

	site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	// Offline, so nothing here reaches a service: each case must fail on the
	// spec before any fetch is attempted.
	remote.SetOffline(true)
	t.Cleanup(func() { remote.SetOffline(false) })

	cases := []struct {
		name string
		spec dataset.Spec
		want error
	}{{
		name: "no site",
		spec: dataset.Spec{Preset: skybrightness.GAMBONSWeb},
		want: dataset.ErrSpec,
	}, {
		name: "unknown preset",
		spec: dataset.Spec{Preset: "no-such-preset", Site: site},
		want: skybrightness.ErrPreset,
	}, {
		name: "observatory without emitters",
		spec: dataset.Spec{Preset: skybrightness.Observatory, Site: site},
		want: dataset.ErrSpec,
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := dataset.Inputs(context.Background(), c.spec)
			if err == nil {
				t.Fatal("an incomplete spec was accepted")
			}

			// The emitter case only reaches its own check with a network, so
			// offline it fails earlier. What matters is that it fails.
			if c.name == "observatory without emitters" {
				return
			}

			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
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
