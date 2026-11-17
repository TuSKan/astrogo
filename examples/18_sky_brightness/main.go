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

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/eop"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
	"github.com/TuSKan/astrogo/time"
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
	site, err := coord.NewEarthLocation(-24.6272, -70.4045, 2635)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	// A moonless night. GAMBONSWeb has no moonlight term at all, so the date
	// matters here only through the zodiacal light's solar elongation.
	when := time.GoDate(2026, 3, 20, 5, 0, 0, 0, time.LocationUTC)

	// Consent, granted explicitly and only for what this preset fetches.
	// Nothing in astrogo downloads a file without it.
	ids, size := dataset.Endpoints(preset)
	remote.EnableDownloads(size, ids...)
	// Earth orientation, for where the site is on the turning Earth:
	// the IERS bulletin, which the remote/eop import above fetches.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	show.Title("How dark is the sky?",
		fmt.Sprintf("%s at Cerro Paranal, %s UTC", preset, when.Format("2006-01-02 15:04")))
	fmt.Println()
	show.Note("Fetching reference data (the first run downloads about 145 MB)...")

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

	// The sky toward the north, from the zenith down. Azimuth 0 throughout,
	// so what changes between rows is the altitude and what lies there.
	byAltitude := show.NewTable(show.Right("Altitude"), show.Right("Sky brightness"))

	var zenith *skybrightness.Estimate

	for _, altDeg := range []float64{90, 60, 30, 15} {
		est, err := sky.Direction(ctx, scene, angle.Deg(altDeg), angle.Deg(0))
		if err != nil {
			log.Fatalf("estimate at %g degrees: %v", altDeg, err)
		}

		sb, err := sky.SurfaceBrightness(est)
		if err != nil {
			log.Fatalf("surface brightness: %v", err)
		}

		byAltitude.Row(fmt.Sprintf("%.0f°", altDeg), fmt.Sprintf("%.2f mag/arcsec²", sb))

		if altDeg == 90 {
			zenith = est
		}
	}

	show.Section("From the zenith down")
	byAltitude.Print()
	fmt.Println()
	show.Note("These do not fall off smoothly with altitude, and that is the point:",
		"the night sky is not a uniform dome. Airmass grows toward the horizon,",
		"which brightens it, while starlight and zodiacal light depend on where",
		"this azimuth happens to be pointing. The two compete, and a model that",
		"only knew about airmass would miss most of what is going on.")

	composition(sky, zenith)
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

	tab := show.NewTable(show.Left("Component"), show.Right("mag/arcsec²"), show.Right("Share"))
	for _, r := range rows {
		tab.Row(string(r.Component), fmt.Sprintf("%.2f", r.Brightness), fmt.Sprintf("%.1f%%", 100*r.Fraction))
	}

	show.Section("The zenith, by component")
	tab.Print()
	fmt.Println()
	show.Note("The shares add to 100%; the magnitudes do not add to anything,",
		"since the components sum in linear radiance.")
}
