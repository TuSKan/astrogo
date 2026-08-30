// Does the model matter? Does the air?
//
// Two comparisons over one night, each changing exactly one thing. A table
// that varied the preset and the place at once could not say which caused the
// difference, which is the failure mode this example exists to avoid.
//
// Run it:
//
//	go run ./examples/25_sky_brightness_compare
//
// It needs the same ~145 MB of reference data as examples/18_sky_brightness
// and shares a cache with it, so run that one first if you would rather the
// download happened somewhere less surprising.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// Cerro Paranal. The coordinates stay fixed through both comparisons below,
// including the one about sites: moving them would change which part of the
// sky is overhead, and the second table would stop being attributable.
const (
	lonDeg   = -70.4045
	latDeg   = -24.6272
	heightM  = 2635
	azimuthD = 0
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// A moonless night. None of the presets used here has a moonlight term,
	// so the date reaches the answer only through the zodiacal light's solar
	// elongation.
	when := time.GoDate(2026, 3, 20, 5, 0, 0, 0, time.LocationUTC)

	// Observatory reads everything the natural presets do and a solar
	// spectrum besides, so granting for it covers the whole run.
	ids, size := dataset.Endpoints(skybrightness.Observatory)
	remote.EnableDownloads(size, ids...)

	fmt.Println("Fetching reference data (first run downloads ~145 MB)...")

	comparePresets(ctx, when)
	compareAir(ctx, when)
	whatObservatoryAdds(ctx)
}

// comparePresets runs three models of the natural sky over one scene.
//
// The expected result is that almost nothing happens, which is why it is
// worth printing: the interesting fact about the transfer choice is how
// little it buys at the zenith.
func comparePresets(ctx context.Context, when time.GoTime) {
	site := siteAt(heightM)
	air := atmosphere.RuralAerosol(site.Height(), atmosphere.CleanMountainAOD550)

	fmt.Printf("\nOne site, one night, three models of the same natural sky\n")
	fmt.Printf("Cerro Paranal, %s UTC, clean mountain air\n\n", when.Format("2006-01-02 15:04"))
	fmt.Printf("  %-14s %12s %14s\n", "preset", "zenith", "10° altitude")

	presets := []skybrightness.Preset{
		skybrightness.GAMBONSWeb,
		skybrightness.NaturalSky,
		skybrightness.GAMBONSFull,
	}

	zeniths := make([]float64, 0, len(presets))

	for _, p := range presets {
		// One Open per preset. Nothing a Sky holds depends on the observer,
		// but the component set and the transfer do, so a preset needs its
		// own.
		sky, err := dataset.Open(ctx, dataset.Spec{Preset: p})
		if err != nil {
			log.Fatalf("open %s: %v", p, err)
		}

		scene, err := sky.Scene(site, when, air)
		if err != nil {
			log.Fatalf("scene for %s: %v", p, err)
		}

		zenith := brightness(ctx, sky, scene, 90)
		zeniths = append(zeniths, zenith)

		fmt.Printf("  %-14s %8.3f mag %10.3f mag\n", p, zenith, brightness(ctx, sky, scene, 10))
	}

	fmt.Printf(`
They agree to %.3f mag at the zenith, and the way they disagree is the finding.

Masana et al. (2024) say the effective-depth transfer behind their web service
cannot exactly reproduce the Eq. 11 integral, and that it runs bright near the
horizon and dark at the zenith by under a tenth of a magnitude for most cases.
Both halves of that are in the table: gambons-web is the fainter of the two at
the zenith and the brighter of the two at 10 degrees, so the sign of the gap
flips between the columns, and neither gap reaches a twentieth of a magnitude.

So anyone quoting a sky brightness to a tenth of a magnitude may take whichever
is cheapest — and gambons-web is about a thousand times cheaper per direction
than gambons-full, which runs the hemispheric integral for every direction
asked of it. natural-sky is those same components again at Duriscoe's kappa
rather than the web service's, and comes out faintest throughout because the
higher kappa attenuates the diffuse terms harder.

Observatory is absent on purpose. It adds moonlight and artificial skyglow, so
it would be a different sky rather than a different transfer, and it needs a
ground-emitter inventory this example does not build.
`, spread(zeniths))
}

// compareAir runs one model over three atmospheres.
//
// Elevation and aerosol together, because they are not independent in
// practice: the sites that are high are the sites that are dry.
func compareAir(ctx context.Context, when time.GoTime) {
	// One Sky for every site here. Nothing it holds depends on the observer —
	// the star and dust maps are all-sky, the grid and passband are spectral
	// — so the site arrives with the scene instead.
	sky, err := dataset.Open(ctx, dataset.Spec{Preset: skybrightness.GAMBONSWeb})
	if err != nil {
		log.Fatalf("open: %v", err)
	}

	fmt.Printf("\nOne model, one night, three atmospheres\n")
	fmt.Printf("The same coordinates each time, so only the air differs\n\n")
	fmt.Printf("  %-22s %6s %7s %10s %13s\n",
		"air", "height", "AOD550", "zenith", "10° altitude")

	for _, row := range []struct {
		name    string
		heightM float64
		build   func(heightM, aod550 float64) *atmosphere.Builder
		aod550  float64
	}{
		{"mountain, clean", 2635, atmosphere.RuralAerosol, atmosphere.CleanMountainAOD550},
		{"lowland, continental", 500, atmosphere.RuralAerosol, atmosphere.ContinentalAOD550},
		{"sea level, urban", 0, atmosphere.UrbanAerosol, atmosphere.UrbanAOD550},
	} {
		site := siteAt(row.heightM)

		scene, err := sky.Scene(site, when, row.build(row.heightM, row.aod550))
		if err != nil {
			log.Fatalf("scene for %s: %v", row.name, err)
		}

		fmt.Printf("  %-22s %5.0fm %7.2f %6.3f mag %9.3f mag\n",
			row.name, row.heightM, row.aod550,
			brightness(ctx, sky, scene, 90), brightness(ctx, sky, scene, 10))
	}

	fmt.Print(`
That ordering runs backwards, and it is not a bug.

None of the natural presets models artificial light at all — GAMBONS describes
a moonless night with no lamps in it. So the only thing more air and more
aerosol can do here is extinguish starlight and zodiacal light faster than the
diffuse term scatters them back, and the hazy sea-level sky comes out the
darker of the three.

That is correct physics and a trap. Read as advice about where to put a
telescope it says the opposite of the truth. Getting the number a city dweller
would recognise needs the Observatory preset over a real ground-emitter
inventory, and that inventory is the one input satellite radiance alone cannot
supply: the same VIIRS pixel is produced by many different real installations,
differing in spectrum and in how much light they throw sideways rather than up.

There is a third way the answer moves with the site, held fixed here on
purpose. Different coordinates put a different part of the sky overhead, and
starlight and zodiacal light follow it. See examples/18_sky_brightness, which
sweeps altitude at one site and shows the same effect along one azimuth.
`)
}

// whatObservatoryAdds decomposes one sky under this module's own model.
//
// Deliberately not a fourth row in the first table. Observatory changes the
// component set as well as the transfer, so as a preset alongside the other
// three it would vary two things at once — and it would vary them loudly,
// since a Moon near full is worth about four magnitudes. A reader would
// attribute that to the better model. A composition is single-model and has
// nothing to confound: this is what one sky is made of, not a comparison.
func whatObservatoryAdds(ctx context.Context) {
	// A night with the Moon high and nearly full. The term this section
	// exists to show is exactly zero when it is not, and a column of zeros
	// looks in every way like coverage — a mistake this module's own golden
	// fixture made once already.
	when := time.GoDate(2026, 4, 2, 5, 0, 0, 0, time.LocationUTC)
	site := siteAt(heightM)

	sky, err := dataset.Open(ctx, dataset.Spec{
		Preset:   skybrightness.Observatory,
		Emitters: []skybrightness.GroundEmitter{illustrativeTown()},
	})
	if err != nil {
		log.Fatalf("open observatory: %v", err)
	}

	scene, err := sky.Scene(site, when,
		atmosphere.RuralAerosol(site.Height(), atmosphere.CleanMountainAOD550))
	if err != nil {
		log.Fatalf("scene: %v", err)
	}

	est, err := sky.Zenith(ctx, scene)
	if err != nil {
		log.Fatalf("zenith: %v", err)
	}

	rows, err := sky.Composition(est)
	if err != nil {
		log.Fatalf("composition: %v", err)
	}

	total, err := sky.SurfaceBrightness(est)
	if err != nil {
		log.Fatalf("surface brightness: %v", err)
	}

	fmt.Printf("\nWhat this module's own model adds\n")
	fmt.Printf("The zenith at Cerro Paranal, %s UTC, Moon high and near full\n\n",
		when.Format("2006-01-02 15:04"))

	var moonShare, artificialShare float64

	for _, r := range rows {
		fmt.Printf("  %-20s %6.2f mag %6.2f%%\n", r.Component, r.Brightness, 100*r.Fraction)

		if r.Component == skybrightness.Moonlight {
			moonShare = 100 * r.Fraction
		}

		if r.Component == skybrightness.Artificial {
			artificialShare = 100 * r.Fraction
		}
	}

	fmt.Printf("  %-20s %6.2f mag\n", "total", total)

	fmt.Printf(`
Moonlight is %.1f per cent of that sky. It is the largest capability
difference between this preset and GAMBONS, which models a moonless night and
has no term for it at all, and it is why Observatory has no published
counterpart to be validated against. The natural components are unchanged in
kind from the tables above — they are simply swamped, and comparing this total
against those would be comparing two different skies.

Artificial skyglow is the other addition, and here it is %.2f per cent: a town
120 km away barely reaches a site chosen for being far from towns, which is
the correct answer for Paranal and the reason it was worth running rather than
asserting. That term is worth exactly as much as the inventory behind it, and
this inventory is a stated estimate — see illustrativeTown for which half of
it is measurable and which half no satellite can supply.
`, moonShare, artificialShare)
}

// illustrativeTown stands in for the nearest town, about 120 km north — which
// is roughly where Antofagasta lies from Paranal.
//
// # Where its two numbers come from, and why only one of them is a guess
//
// The magnitude is estimated rather than invented. VIIRS reports a modest
// town at something like 20 nW cm^-2 sr^-1 in its day-night band, which is
// 2e-4 W m^-2 sr^-1, and spreading that across the band's ~400 nm gives the
// 5e-7 W m^-2 sr^-1 nm^-1 below. That chain is sound as far as it goes,
// because the day-night band looks straight down and this field is radiance
// leaving the source upward, so the two are the same quantity.
//
// The *shape* is the guess, and it is the one no satellite can supply. Garstang's
// Q = q = 0.15 here are the values Kocifaj (2007) uses in its own numerical
// runs, so they are cited rather than made up — but nothing measured chose
// them for this town, and near-horizontal emission is what carries skyglow to
// distance. That gap is exactly the one docs/skybrightness.md §16 records
// against `dataset/viirs`: the same satellite pixel is produced by many real
// installations differing in how much light they throw sideways rather than
// up, so the absolute scale is reachable and the angular distribution is not.
//
// Flagging is what makes using it acceptable rather than a fabrication.
// AssumedSourceSpectrum and AssumedEmissionFunction reach the estimate's
// Quality, so the assumption travels with the result instead of being lost
// between here and whoever reads the number.
func illustrativeTown() *skybrightness.UniformEmitter {
	const (
		distanceKM = 120
		degPerKM   = 1 / 111.195
	)

	at, err := coord.NewGeodetic(
		angle.Deg(lonDeg), angle.Deg(latDeg+distanceKM*degPerKM), 50)
	if err != nil {
		log.Fatalf("town: %v", err)
	}

	// 20 nW cm^-2 sr^-1 over the day-night band's ~400 nm. Flat across
	// wavelength, which no real inventory is — a sodium town is not a LED one
	// — and another reason the flags below are not decoration.
	const radiance = 5e-7

	return &skybrightness.UniformEmitter{
		At:           at,
		Name:         "illustrative town",
		WavelengthNM: []unit.WavelengthNM{300, 400, 500, 600, 700, 800, 1100},
		Radiance: []float64{
			radiance, radiance, radiance, radiance, radiance, radiance, radiance,
		},
		Emission: skybrightness.GarstangEmission{
			ReflectedFraction: 0.15,
			DirectFraction:    0.15,
		},
		Flags: skybrightness.AssumedSourceSpectrum | skybrightness.AssumedEmissionFunction,
	}
}

// brightness evaluates one direction and projects it through the Sky's own
// passband and magnitude system.
func brightness(
	ctx context.Context, sky *dataset.Sky, scene *skybrightness.Scene, altDeg float64,
) float64 {
	est, err := sky.Direction(ctx, scene, angle.Deg(altDeg), angle.Deg(azimuthD))
	if err != nil {
		log.Fatalf("estimate at %g degrees: %v", altDeg, err)
	}

	sb, err := sky.SurfaceBrightness(est)
	if err != nil {
		log.Fatalf("surface brightness at %g degrees: %v", altDeg, err)
	}

	return sb
}

// siteAt is the fixed coordinates at a chosen elevation.
func siteAt(h float64) *coord.Geodetic {
	site, err := coord.NewGeodetic(angle.Deg(lonDeg), angle.Deg(latDeg), h)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	return site
}

// spread is the range of a set of magnitudes.
func spread(v []float64) float64 {
	lo, hi := v[0], v[0]

	for _, x := range v[1:] {
		lo = min(lo, x)
		hi = max(hi, x)
	}

	return hi - lo
}
