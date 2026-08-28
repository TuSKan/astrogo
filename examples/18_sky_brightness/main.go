// How dark is the sky here tonight?
//
// Predicts the night-sky surface brightness at a site, in mag/arcsec^2, from
// the zenith down toward the horizon, and says what the sky is made of.
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
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
)

// The preset to run. GAMBONSWeb reproduces the GAMBONS web service: the five
// natural components of a moonless night under a simplified transfer, which
// is the cheapest of the four and the one with a published number to check
// against.
//
// NaturalSky is the same physics at Duriscoe's transfer factor. GAMBONSFull
// runs the full scattering integral and costs about a thousand times more per
// direction. Observatory adds moonlight and artificial skyglow, and needs a
// ground-emitter inventory this example does not build.
//
// Nothing else here changes when this does: the preset carries its own
// transfer, fidelity, passband and grid, and dataset.Sky applies them.
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
	// matters here only through the zodiacal light's solar elongation.
	when := time.Date(2026, 3, 20, 5, 0, 0, 0, time.UTC)

	// Consent, granted explicitly and only for what this preset fetches.
	// Nothing in astrogo downloads a file without it.
	ids, size := dataset.Endpoints(preset)
	remote.EnableDownloads(size, ids...)

	fmt.Println("Fetching reference data (first run downloads ~145 MB)...")

	sky, err := dataset.Open(ctx, dataset.Spec{Preset: preset})
	if err != nil {
		log.Fatalf("open: %v", err)
	}

	// The night's own air: the site's elevation, which sets surface pressure
	// and temperature from the standard profile, and how much aerosol is
	// overhead. The vertical profile comes with the aerosol type.
	//
	// The optical properties that go with "rural" — single-scattering albedo,
	// asymmetry, Angstrom exponent — come from OPAC (Hess, Koepke & Schult
	// 1998) rather than from this file. The optical depth cannot: it is how
	// much aerosol is above this site tonight, and varies by an order of
	// magnitude across a year. CleanMountainAOD550 is a stated starting point
	// for a high dry site; dataset.LiveAerosol fetches the real figure for a
	// place and an hour from Copernicus, which is the same call with the
	// guess taken out.
	air := atmosphere.RuralAerosol(site.Height(), atmosphere.CleanMountainAOD550)

	scene, err := sky.Scene(site, when, air)
	if err != nil {
		log.Fatalf("scene: %v", err)
	}

	fmt.Printf("\n%s at Cerro Paranal, %s UTC\n\n", preset, when.Format("2006-01-02 15:04"))
	fmt.Printf("  %-9s  %s\n", "altitude", "sky brightness")

	for _, altDeg := range []float64{90, 60, 30, 15} {
		est, err := sky.Direction(ctx, scene, angle.Deg(altDeg), angle.Deg(0))
		if err != nil {
			log.Fatalf("estimate at %g degrees: %v", altDeg, err)
		}

		sb, err := sky.SurfaceBrightness(est)
		if err != nil {
			log.Fatalf("surface brightness: %v", err)
		}

		fmt.Printf("  %6.0f°     %6.2f mag/arcsec²\n", altDeg, sb)

		if altDeg == 90 {
			composition(sky, est)
		}
	}

	fmt.Println("\nThese do not fall off smoothly with altitude, and that is the point:")
	fmt.Println("the night sky is not a uniform dome. Airmass grows toward the horizon,")
	fmt.Println("which brightens it, while starlight and zodiacal light depend on where")
	fmt.Println("this azimuth happens to be pointing. The two compete, and a model that")
	fmt.Println("only knew about airmass would miss most of what is going on.")
}

// composition prints what the zenith sky is made of.
//
// Worth seeing once: the total is a sum in linear radiance, so the shares add
// to one while the magnitudes beside them do not add to anything. Which term
// leads changes with where you point and how dark the site is.
func composition(sky *dataset.Sky, est *skybrightness.Estimate) {
	rows, err := sky.Composition(est)
	if err != nil {
		log.Fatalf("composition: %v", err)
	}

	fmt.Println()

	for _, r := range rows {
		fmt.Printf("      %-20s %6.2f  %5.1f%%\n",
			r.Component, r.Brightness, 100*r.Fraction)
	}

	fmt.Println()
}
