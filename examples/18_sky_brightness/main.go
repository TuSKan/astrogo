// How dark is the sky here tonight?
//
// Predicts the night-sky surface brightness at a site, in mag/arcsec^2, and
// prints it from the zenith down to the horizon along with the breakdown that
// produced it.
//
// Run it:
//
//	go run ./examples/18_sky_brightness
//
// The first run downloads about 145 MB of reference data — the integrated
// starlight map and both hemispheres of the SFD dust map — and caches it.
// Later runs need the network only for the airglow spectrum and the passband.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
)

// The preset to run. GAMBONSWeb reproduces the GAMBONS web service: the five
// natural components of a moonless night under a simplified transfer, which is
// the cheapest of the four and the one with a published number to check
// against.
//
// NaturalSky is the same physics at Duriscoe's transfer factor. GAMBONSFull
// runs the full scattering integral and costs about a thousand times more per
// direction. Observatory adds moonlight and artificial skyglow, and needs a
// ground-emitter inventory this example does not build.
const preset = skybrightness.GAMBONSWeb

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Cerro Paranal: 2,635 m, and about as dark as a site gets.
	site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	// A moonless night. GAMBONSWeb has no moonlight term at all, so the date
	// only matters here through the zodiacal light's solar elongation.
	when := time.Date(2026, 3, 20, 5, 0, 0, 0, time.UTC)

	// Consent, granted explicitly and only for what this needs. Nothing in
	// astrogo downloads a file without it.
	ids, size := dataset.Endpoints(preset)
	remote.EnableDownloads(size, ids...)

	fmt.Println("Fetching reference data (first run downloads ~145 MB)...")

	in, err := dataset.Inputs(ctx, dataset.Spec{Preset: preset, Site: site})
	if err != nil {
		log.Fatalf("inputs: %v", err)
	}

	model, err := skybrightness.NewPreset(preset, in)
	if err != nil {
		log.Fatalf("model: %v", err)
	}

	scene, err := buildScene(site, when)
	if err != nil {
		log.Fatalf("scene: %v", err)
	}

	fidelity, err := preset.Fidelity()
	if err != nil {
		log.Fatalf("fidelity: %v", err)
	}

	fmt.Printf("\n%s at Cerro Paranal, %s UTC\n\n", preset, when.Format("2006-01-02 15:04"))
	fmt.Printf("  %-10s  %-18s\n", "altitude", "sky brightness")

	for _, altDeg := range []float64{90, 60, 30, 15} {
		est, err := model.Estimate(ctx, skybrightness.Query{
			Scene:     scene,
			Direction: coord.NewAltAz(angle.Deg(altDeg), angle.Deg(0)),
			Grid:      in.Grid,
			Fidelity:  fidelity,
		})
		if err != nil {
			log.Fatalf("estimate at %g degrees: %v", altDeg, err)
		}

		sb, err := est.SurfaceBrightness(in.Band, magnitude.Vega)
		if err != nil {
			log.Fatalf("surface brightness: %v", err)
		}

		fmt.Printf("  %6.0f°     %6.2f mag/arcsec²\n", altDeg, sb)

		if altDeg == 90 {
			breakdown(est, in)
		}
	}

	fmt.Println("\nThese do not fall off smoothly with altitude, and that is the point:")
	fmt.Println("the night sky is not a uniform dome. Airmass grows toward the horizon,")
	fmt.Println("which brightens it, while starlight and zodiacal light depend on where")
	fmt.Println("this azimuth happens to be pointing. The two compete, and a model that")
	fmt.Println("only knew about airmass would miss most of what is going on.")
}

// buildScene assembles the physical state the model evaluates under.
//
// The atmosphere carries the preset's own transfer factor and scattering
// order. Those are not decoration: a scene that disagrees with the preset it
// is evaluated against is a different model wearing the same name, and
// Estimate rejects it rather than returning a plausible number.
func buildScene(site *coord.Geodetic, when time.Time) (*skybrightness.Scene, error) {
	kappa, err := preset.DiffuseKappa()
	if err != nil {
		return nil, fmt.Errorf("preset kappa: %w", err)
	}

	multiple, err := preset.MultipleScattering()
	if err != nil {
		return nil, fmt.Errorf("preset scattering order: %w", err)
	}

	air, err := atmosphere.NewBuilder().
		Surface(743, 284).                   // hPa, K
		Aerosol(0.02, 550, 1.3, 0.95, 0.65). // a clean, dry night
		BoundaryLayer(1500).                 // metres
		DiffuseScattering(kappa).
		MultipleScattering(multiple).
		Build()
	if err != nil {
		return nil, fmt.Errorf("atmosphere: %w", err)
	}

	return &skybrightness.Scene{
		Observer:   site,
		Time:       when,
		Atmosphere: air,
		Ephemeris:  eph.Default(),
	}, nil
}

// breakdown prints what each component contributed at 550 nm.
//
// Worth seeing once: the total is a sum in linear radiance, and which term
// dominates changes with where you point and how dark the site is. At a good
// site on a moonless night airglow and zodiacal light lead, and starlight
// carries the structure.
func breakdown(est *skybrightness.Estimate, in skybrightness.PresetInputs) {
	const nm550 = 550

	idx := 0

	for i := range in.Grid.Len() {
		if float64(in.Grid.At(i)) >= nm550 {
			idx = i

			break
		}
	}

	fmt.Println("\n             at 550 nm, W m^-2 sr^-1 nm^-1:")

	for _, id := range est.ComponentIDs() {
		spec, ok := est.Component(id)
		if !ok {
			continue
		}

		fmt.Printf("               %-20s %.3e\n", id, spec[idx])
	}

	fmt.Println()
}
