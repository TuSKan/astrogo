// Example: Sky brightness and limiting magnitude under moonlight.
//
// This pairs the plan event API with the skybrightness model:
//   - plan.AstronomicalDawnDusk frames the true-night window,
//   - plan.MoonIllumination / plan.MoonriseMoonset describe the Moon,
//   - skybrightness/atlas.Resolver discovers the site's artificial
//     light-pollution floor via LayerAuto (the default): real downloaded
//     data — VIIRS (newest published year) first, then the World Atlas —
//     when download consent has been granted, otherwise a fixed
//     Bortle-class estimate. Grant the two remote.EnableDownloads calls
//     below to see a real layer answer instead of the fallback.
//   - a skybrightness.CompositeModel (airglow + zodiacal light, the resolved
//     floor, plus scattered moonlight) gives the sky surface brightness
//     toward a pointing, and
//   - skybrightness.VisualLimitingMag turns that into a limiting magnitude.
//
// Finally plan.ScoreObservableSky shows the LimitingMagnitudeConstraint
// demoting a target's observability score under the moonlit sky.
package main

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/atlas"
	"github.com/TuSKan/astrogo/skybrightness/lpmap"
	"github.com/TuSKan/astrogo/time"
)

const (
	// fallbackBortleClass is the estimate LayerAuto settles on when no
	// download consent has been granted for the real atlas layers —
	// class 4 is the suburban/rural transition.
	fallbackBortleClass = 4
	// pointingAltitude is the altitude both sample pointings use, high
	// enough above the horizon that airmass and extinction stay modest.
	pointingAltitude = 50.0
	// targetVMag is the V magnitude of the demo target scored at the end.
	targetVMag = 5.5
)

func main() {
	ctx := context.Background()

	// JPL kernel downloads are opt-in — see README "Data downloads &
	// offline usage". de442 is ~115 MB; naif0012.tls (leap seconds) ~5 KB.
	remote.EnableDownloads(remote.NAIFSPK, 200<<20)
	remote.EnableDownloads(remote.NAIFLSK, 0)

	// The two atlas layers LayerAuto tries. Size the caps to the real
	// archives or the download is denied on size alone: World Atlas 2015
	// is ~653 MB, and a VIIRS annual composite runs ~700 MB-1 GB (2025 is
	// ~928 MB).
	remote.EnableDownloads(remote.WorldAtlas, 700<<20)
	remote.EnableDownloads(remote.VIIRSAnnual, 1000<<20)

	// No consent call for the lightpollutionmap.info live API: it needs a
	// manually-issued key in LIGHTPOLLUTIONMAP_KEY, not a download budget.
	// Without one its column below reads "—" and the rest still works.

	// ── Observatory: Quinta Calixto ───────────────────────────────────────
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}

	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.5190,-46.4673, 835.05, plan.WithTimeZone(tz))
	if err != nil {
		log.Fatalf("build site: %v", err)
	}

	provider, err := eph.NewProvider(ctx, eph.Planets, "de442")
	if err != nil {
		log.Fatalf("open DE442 ephemeris: %v", err)
	}

	// Night of the 2025-03-14 full Moon (local afternoon → next afternoon).
	start := time.Date(2025, 3, 14, 12, 0, 0, 0, tz)
	end := start.AddDate(0, 0, 1)

	// ── Night circumstances via the plan event API ───────────────────────
	dawn, dusk, err := plan.AstronomicalDawnDusk(start, end, site, provider)
	if err != nil {
		log.Fatalf("astronomical twilight: %v", err)
	}

	frac, phase, err := plan.MoonIllumination(start.Add(12*time.Hour), provider)
	if err != nil {
		log.Fatalf("moon illumination: %v", err)
	}

	moonrise, moonset, err := plan.MoonriseMoonset(start, end, site, provider)
	if err != nil {
		log.Fatalf("moonrise/moonset: %v", err)
	}

	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Printf("  Sky Brightness & Limiting Magnitude — %s\n", site.Name())
	fmt.Printf("  Moon: %.0f%% illuminated (phase angle %.0f°)\n", frac*100, phase.Degrees())
	printEvent("  Astronomical dusk", dusk, tz)
	printEvent("  Astronomical dawn", dawn, tz)
	printEvent("  Moonrise         ", moonrise, tz)
	printEvent("  Moonset          ", moonset, tz)
	fmt.Println("══════════════════════════════════════════════════════════════")

	// Observe two hours after moonrise, when the Moon is comfortably up.
	obs := start.Add(15 * time.Hour)
	if moonrise != nil {
		obs = moonrise.Time.Add(2 * time.Hour)
	}

	// skyCtx caches the epoch's astrometric transform — distinct from the
	// context.Context above; reused by every pointing evaluated below.
	skyCtx := coord.NewContext(obs, site.Location(), site.Atmosphere())

	moonPos, err := plan.NewMoon(provider).Position(obs)
	if err != nil {
		log.Fatalf("moon position: %v", err)
	}

	moonAA, err := skyCtx.ICRSToAltAz(moonPos)
	if err != nil {
		log.Fatalf("moon alt/az: %v", err)
	}

	fmt.Printf("\n  Observing %s — Moon at altitude %.0f°\n",
		obs.In(tz).Format("Jan 02 15:04 MST"), moonAA.Alt().Degrees())

	// ── Assemble the sky model ──────────────────────────────────────────
	// Airglow and zodiacal light are physical baselines that assume
	// nothing about the site; the artificial floor is discovered.
	components := make([]skybrightness.Component, 0, 3) // airglow, zodiacal, floor
	components = append(
		components,
		skybrightness.NewAirglow(),
		skybrightness.NewZodiacalLight(provider),
	)

	// One call: LayerAuto is freshness-first — VIIRS (newest published
	// year), then the 2015-frozen World Atlas, then the Bortle fallback —
	// and reports what it tried in Result.Attempts.
	floor, err := atlas.FloorAt(ctx, site.Location(), atlas.WithBortleClass(fallbackBortleClass))
	if err != nil {
		log.Fatalf("resolve light-pollution floor: %v", err)
	}

	fmt.Printf("\n  %s light-pollution floor: %.2f V mag/arcsec² via %s (%s)\n",
		site.Name(), float64(floor.SQM), floor.Layer, floor.Source)

	for _, attempt := range floor.Attempts {
		if attempt.Err != nil {
			fmt.Printf("    (%s unavailable: %v)\n", attempt.Layer, attempt.Err)
		}
	}

	components = append(components, floor.Floor)

	// Clone before appending: NewCompositeModel keeps the slice it is given
	// rather than copying it, so appending to components in place would hand
	// the two models overlapping backing arrays.
	natural := skybrightness.NewCompositeModel(components...)
	full := skybrightness.NewCompositeModel(append(slices.Clone(components), skybrightness.NewMoonlight())...)
	conv := skybrightness.NewVisualLimitingMag()

	fmt.Println("\n── Sky surface brightness (V mag/arcsec², larger = darker) ────")
	fmt.Printf("  %-16s %8s %8s %7s  %s\n", "Pointing", "Natural", "Full", "LimMag", "Equivalent sky")
	reportPointing("  toward Moon", coord.NewAltAz(angle.Deg(pointingAltitude), moonAA.Az()), skyCtx, natural, full, conv)
	reportPointing("  away from Moon", coord.NewAltAz(angle.Deg(pointingAltitude), moonAA.Az().Add(angle.Deg(180))), skyCtx, natural, full, conv)

	// ── Constraint: moonlight demotes a target's observability score ─────
	scoreTarget(site, obs, skyCtx, natural, full, conv)

	// ── The same question asked of every source ──────────────────────────
	compareLayers(ctx)
}

// comparisonSites spans the full dynamic range the layers have to cover, from
// a megacity core to two of the darkest professional sites on Earth. Ordering
// the table by expected brightness makes a decoding or georeferencing error
// obvious: any source that scrambles this ordering is wrong regardless of how
// plausible its individual numbers look.
var comparisonSites = []struct {
	name           string
	latDeg, lonDeg float64
}{
	{"São Paulo (centre)", -23.5505, -46.6333},
	{"London (centre)", 51.5074, -0.1278},
	{"Quinta Calixto", -22.528478, -46.473002},
	{"Mauna Kea", 19.8207, -155.4681},
	{"Paranal (VLT)", -24.6275, -70.4044},
}

// compareLayers resolves the same five sites through each layer separately, so
// the sources can be read against each other rather than through LayerAuto's
// single winner.
//
// One Resolver per layer, reused across every site — the atlas layers hold a
// multi-gigabyte file open, so building one per (site, layer) would reopen and
// re-validate it 5 times over. This is exactly what NewResolver exists for and
// what FloorAt (build, ask once, release) is deliberately not for.
func compareLayers(ctx context.Context) {
	layers := []struct {
		label string
		opts  []atlas.Option
	}{
		{"VIIRS 2025", []atlas.Option{atlas.WithLayer(atlas.LayerVIIRS)}},
		{"WA 2015", []atlas.Option{atlas.WithLayer(atlas.LayerWorldAtlas)}},
		{"LPmap API", []atlas.Option{
			atlas.WithLayer(atlas.LayerLightPollutionMap),
			atlas.WithLightPollutionMap(lpmap.New()),
		}},
	}

	// cells[layer][site], plus the first failure per layer to explain a
	// column of dashes once instead of five times.
	cells := make([][]string, len(layers))
	notes := make([]string, len(layers))

	for i, l := range layers {
		// WithQuiet: per-download progress logging would shred a table
		// that resolves fifteen values.
		resolver := atlas.NewResolver(append(slices.Clone(l.opts), atlas.WithQuiet())...)
		cells[i] = make([]string, len(comparisonSites))

		for j, s := range comparisonSites {
			loc, err := coord.NewEarthLocation(s.latDeg, s.lonDeg, 0)
			if err != nil {
				log.Fatalf("build %s location: %v", s.name, err)
			}

			result, err := resolver.Floor(ctx, loc)
			if err != nil {
				cells[i][j] = "—"

				if notes[i] == "" {
					notes[i] = fmt.Sprintf("%s: %v", l.label, err)
				}

				continue
			}

			cells[i][j] = formatFloor(result.SQM)
		}

		// Closed as soon as this layer's column is done rather than
		// deferred, so at most one multi-gigabyte atlas file is open at a
		// time instead of all three.
		if err := resolver.Close(); err != nil {
			log.Fatalf("close %s resolver: %v", l.label, err)
		}
	}

	fmt.Println("\n── Artificial light-pollution floor by source (mcd/m², 0 = pristine) ──")
	fmt.Printf("  %-20s", "Site")

	for _, l := range layers {
		fmt.Printf(" %10s", l.label)
	}

	fmt.Println()

	for j, s := range comparisonSites {
		fmt.Printf("  %-20s", s.name)

		for i := range layers {
			fmt.Printf(" %10s", cells[i][j])
		}

		fmt.Println()
	}

	for _, n := range notes {
		if n != "" {
			fmt.Printf("    (unavailable — %s)\n", n)
		}
	}

	fmt.Println("    VIIRS 0 is a measured zero, not a missing value: the day-night")
	fmt.Println("    band floors at zero below its detection limit, so every truly dark")
	fmt.Println("    site reads alike (confirmed against lightpollutionmap.info, which")
	fmt.Println("    reports 0.00 nW/cm²·sr at Mauna Kea for every year 2012-2025).")
	fmt.Println("    The World Atlas is a propagation model, so it still separates them")
	fmt.Println("    — at the cost of being frozen at 2015.")
}

// formatFloor renders one artificial-floor value for the table, in mcd/m².
//
// The linear unit is the whole point. In magnitudes, "no artificial light" is
// +Inf — arithmetically correct (zero flux) but impossible to tabulate, and
// indistinguishable at a glance from a missing value. VIIRS measures a hard
// zero wherever its day-night band detects nothing, and a measured zero is a
// result, not a gap: mcd/m² prints it as 0.000, ordered against every other
// row, with no sentinel string standing in for a number.
//
// mcd/m² is also the World Atlas's own native unit, so this is the column in
// which the two sources are directly comparable rather than the one where one
// of them overflows.
// Significant digits, not fixed decimals: the values span eight orders of
// magnitude (a megacity core to a modelled 2e-07 at Paranal), so %.2f would
// flatten the entire dark end to "0.00" and re-create exactly the ambiguity
// this format exists to remove. An exact zero is printed bare, so it reads as
// the measurement it is rather than as a very small number.
func formatFloor(sqm skybrightness.SurfaceBrightnessV) string {
	mcd := sqm.McdM2() // +Inf mag ⇒ exactly 0 flux, no special case needed

	if mcd == 0 {
		return "0"
	}

	return fmt.Sprintf("%.3g", mcd)
}

// reportPointing prints the natural-only and full-model sky brightness plus
// the limiting magnitude toward one pointing.
func reportPointing(
	name string, aa coord.AltAz, skyCtx *coord.Context,
	natural, full skybrightness.Model, conv skybrightness.LimitingMagModel,
) {
	naturalSB, err := natural.SurfaceBrightness(aa, skyCtx)
	if err != nil {
		log.Fatalf("%s: natural sky brightness: %v", name, err)
	}

	fullSB, err := full.SurfaceBrightness(aa, skyCtx)
	if err != nil {
		log.Fatalf("%s: full sky brightness: %v", name, err)
	}

	airmass, err := atmosphere.Airmass(aa.Alt())
	if err != nil {
		log.Fatalf("%s: airmass: %v", name, err)
	}

	limMag, err := conv.LimitingMagnitude(fullSB, airmass)
	if err != nil {
		log.Fatalf("%s: limiting magnitude: %v", name, err)
	}

	class, sky := skybrightness.BortleClass(fullSB)

	fmt.Printf("  %-16s %8.2f %8.2f %7.2f  Bortle %d (%s)\n",
		name, float64(naturalSB), float64(fullSB), limMag, class, sky)
}

// scoreTarget scores a fixed-magnitude target near zenith three ways —
// geometry alone, under the natural sky, and under the moonlit sky — so
// the LimitingMagnitudeConstraint's effect is visible as a single number.
func scoreTarget(
	site *plan.Site, obs time.Time, skyCtx *coord.Context,
	natural, full skybrightness.Model, conv skybrightness.LimitingMagModel,
) {
	lst, err := site.LocalSiderealTime(obs)
	if err != nil {
		log.Fatalf("local sidereal time: %v", err)
	}

	star := plan.NewStar(fmt.Sprintf("zenith star (V=%.1f)", targetVMag), lst, site.Latitude())
	required := func(plan.Observable) float64 { return targetVMag }

	base, err := plan.ScoreObservable(star, obs, site, nil, skyCtx)
	if err != nil {
		log.Fatalf("base score: %v", err)
	}

	naturalScore, err := plan.ScoreObservableSky(star, obs, site, nil, skyCtx,
		plan.LimitingMagnitudeConstraint{Model: natural, Conversion: conv, Required: required})
	if err != nil {
		log.Fatalf("natural-sky score: %v", err)
	}

	moonScore, err := plan.ScoreObservableSky(star, obs, site, nil, skyCtx,
		plan.LimitingMagnitudeConstraint{Model: full, Conversion: conv, Required: required})
	if err != nil {
		log.Fatalf("moonlit-sky score: %v", err)
	}

	fmt.Printf("\n── Observability score for a V=%.1f target near zenith ────────\n", targetVMag)
	fmt.Printf("  Base score:            %6.1f\n", base)
	fmt.Printf("  Natural sky score:     %6.1f\n", naturalScore)
	fmt.Printf("  Moonlit sky score:     %6.1f\n", moonScore)
}

// printEvent prints a rise/set/twilight event in local time, or "—" if absent.
func printEvent(label string, e *plan.Event, tz *time.Location) {
	if e == nil {
		fmt.Printf("%s: —\n", label)

		return
	}

	fmt.Printf("%s: %s\n", label, e.Time.In(tz).Format("Jan 02 15:04 MST"))
}
