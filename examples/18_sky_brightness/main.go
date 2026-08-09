// Example: Sky Brightness V2 — spectral sky-radiance evaluation.
//
// This is the mandated end-to-end demonstration for Phase 1
// (docs/skybrightness.md §14): site + time + atmosphere + direction ->
// spectral component decomposition -> passband brightness -> uncertainty
// -> limiting magnitude -> full provenance.
//
// Phase 1 honesty note, stated here and in the printed output: this
// package's ONLY real physics today is the fast, simplified models
// (natural.ConstantAirglow, natural.VBandMoonlight — the
// exact v1 Krisciunas & Schaefer 1991 physics, re-implemented against the
// new spectral API) plus an analytic Rayleigh-only transmission model
// (atmos.RayleighOnly). Real spectral zodiacal light, starlight, diffuse
// galactic light, twilight, and artificial-emission propagation are
// Phase 2-5 scope — see docs/skybrightness.md's staged implementation
// plan. What this example proves is the ENGINE, not yet the full sky.
//
// This pairs the plan event API with the new skybrightness.Engine:
//   - plan.AstronomicalDawnDusk frames the true-night window,
//   - plan.MoonIllumination / plan.MoonriseMoonset describe the Moon,
//   - natural.NewFastEngine (ConstantAirglow + VBandMoonlight,
//     ModeFast) gives the sky's spectral radiance toward a pointing,
//     reduced to a V-band magnitude through natural.TopHatJohnsonV() —
//     the only passband whose output is physically meaningful in Phase 1
//     (see garstang_units.go's doc comment for why),
//   - skybrightness.SchaeferNELM turns that into a limiting magnitude, and
//   - plan.ScoreObservableSky shows LimitingMagnitudeConstraint demoting a
//     target's observability score under the moonlit sky.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/atmos"
	"github.com/TuSKan/astrogo/skybrightness/natural"
	"github.com/TuSKan/astrogo/time"
)

const (
	// pointingAltitude is the altitude the demo pointing uses, high
	// enough above the horizon that airmass stays modest.
	pointingAltitude = 50.0
	// targetVMag is the V magnitude of the demo target scored at the end.
	targetVMag = 5.5
)

func printEvent(label string, e *plan.Event, tz *time.Location) {
	if e == nil {
		fmt.Printf("%s: none in window\n", label)
		return
	}

	fmt.Printf("%s: %s\n", label, e.Time.In(tz).Format("Jan 02 15:04 MST"))
}

func main() {
	ctx := context.Background()

	// JPL kernel downloads are opt-in — see README "Data downloads &
	// offline usage". de442 is ~115 MB; naif0012.tls (leap seconds) ~5 KB.
	remote.EnableDownloads(remote.NAIFSPK, 200<<20)
	remote.EnableDownloads(remote.NAIFLSK, 0)

	// ── Observatory: Quinta Calixto ───────────────────────────────────────
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}

	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.5190, -46.4673, 835.05, plan.WithTimeZone(tz))
	if err != nil {
		log.Fatalf("build site: %v", err)
	}

	provider, err := eph.NewProvider(ctx, eph.Planets, "de442")
	if err != nil {
		log.Fatalf("open DE442 ephemeris: %v", err)
	}

	// Night of the 2025-03-14 full Moon (local afternoon -> next afternoon).
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

	fmt.Println("======================================================================")
	fmt.Printf("  Sky Brightness V2 — %s\n", site.Name())
	fmt.Printf("  Moon: %.0f%% illuminated (phase angle %.0f deg)\n", frac*100, phase.Degrees())
	printEvent("  Astronomical dusk", dusk, tz)
	printEvent("  Astronomical dawn", dawn, tz)
	printEvent("  Moonrise         ", moonrise, tz)
	printEvent("  Moonset          ", moonset, tz)
	fmt.Println("======================================================================")

	// Observe two hours after moonrise, when the Moon is comfortably up.
	obs := start.Add(15 * time.Hour)
	if moonrise != nil {
		obs = moonrise.Time.Add(2 * time.Hour)
	}

	// astro caches the epoch's astrometric transform — ONE per epoch,
	// reused for every pointing evaluated below (the hard repo-wide
	// convention coord.Context itself documents; also see
	// docs/skybrightness.md §5's Request.Astro contract).
	astro := coord.NewContext(obs, site.Location(), site.Refraction())

	moonPos, err := plan.NewMoon(provider).Position(obs)
	if err != nil {
		log.Fatalf("moon position: %v", err)
	}

	moonAA, err := astro.ICRSToAltAz(moonPos)
	if err != nil {
		log.Fatalf("moon alt/az: %v", err)
	}

	fmt.Printf("\n  Observing %s — Moon at altitude %.0f deg\n",
		obs.In(tz).Format("Jan 02 15:04 MST"), moonAA.Alt().Degrees())

	// ── Atmosphere: explicit, user-supplied, immutable — a general
	// atmosphere.Atmosphere (package atmosphere, not skybrightness — the same
	// state a future weather/seeing constraint would use, not something
	// sky-brightness-specific) ────────────────────────────────────────
	atmState, err := atmosphere.NewBuilder().
		Surface(1013.25, 288.15).
		Clear(). // no cloud layers — Phase 1 has no cloud optics yet regardless
		SurfaceAlbedo(atmosphere.UniformAlbedo(0.15)).
		Source(atmosphere.SourceRef{Name: "user-supplied", Fidelity: atmosphere.FidelityMeasured}).
		Build()
	if err != nil {
		log.Fatalf("atmosphere: %v", err)
	}

	// ── Engine: natural.NewFastEngine assembles the fast, offline
	// ConstantAirglow + VBandMoonlight components for us —
	// an application only needs to name its transmission model
	// (docs/skybrightness.md §4) ───────────────────────────────────────
	sky, err := natural.NewFastEngine(natural.FastConfig{
		Ephemeris:    provider,
		Transmission: atmos.NewRayleighOnly(),
	})
	if err != nil {
		log.Fatalf("build engine: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()
	johnsonV := natural.TopHatJohnsonV()

	dir := coord.NewAltAz(angle.Deg(pointingAltitude), angle.Deg(120))

	// A single skybrightness.Point call replaces what used to be a
	// 13-line Engine.Evaluate Request literal plus a manual
	// res.Components.Each+IntegrateRadiance loop — Point now covers
	// transmission and limiting magnitude too, the two derived quantities
	// that previously forced this example back onto Evaluate directly
	// (see docs/skybrightness.md §15's worked example).
	res, err := skybrightness.Point(ctx, sky, skybrightness.PointQuery{
		Astro:               astro,
		Direction:           dir,
		Passband:            johnsonV,
		Mode:                skybrightness.ModeFast,
		Atmosphere:          atmState,
		Grid:                grid,
		Components:          true,
		ComputeTransmission: true,
		LimitingMag:         skybrightness.NewSchaeferNELM(),
	})
	if err != nil {
		log.Fatalf("evaluate: %v", err)
	}

	fmt.Println("\n── Component decomposition (Garstang nanolambert convention, Phase 1) ──")
	fmt.Println("  Meaningful only through TopHatJohnsonV — see the doc comment above.")

	for _, cb := range res.Components {
		fmt.Printf("  %-16s %12.4g  (+/- %.0f%%)  %v\n",
			cb.ID, float64(cb.Radiance), 100*cb.RelSigma, cb.Quality.Strings())
	}

	fmt.Println("\n── Passband brightness ──────────────────────────────────────────────")
	fmt.Printf("  %-16s Vega = %.2f mag/arcsec^2\n", johnsonV.ID, res.Vega)

	fmt.Printf("\n  Atmospheric transmission (Rayleigh-only, Phase 1) at grid ends:\n")

	if len(res.Transmission) >= grid.Len() {
		fmt.Printf("    %.0fnm: %.3f    %.0fnm: %.3f\n",
			float64(grid.At(0)), res.Transmission[0],
			float64(grid.At(grid.Len()-1)), res.Transmission[grid.Len()-1])
	}

	if res.HasLimitingMag {
		fmt.Printf("\n  Limiting magnitude (Schaefer 1990, TopHatJohnsonV): %.2f\n", res.LimitingMagnitude)
	}

	fmt.Printf("  Quality flags: %v\n", res.Quality.Strings())

	// Provenance implements fmt.Stringer directly — no need to reach for
	// encoding/json to get a human-readable summary. A caller wanting the
	// full JSON (for storage/logging) calls json.Marshal(res.Provenance)
	// explicitly; Provenance already implements json.Marshaler.
	fmt.Printf("\n── Provenance ────────────────────────────────────────────────────────\n")
	fmt.Println(res.Provenance)

	// ── Score a target through the moonlit sky ───────────────────────────
	// A real fixed-RA/Dec target that happens to sit at dir right now —
	// found via AltAzToICRS so the scored target and the evaluated
	// pointing are the same real sky position, not just numerically
	// coincident alt/az values.
	targetICRS, err := astro.AltAzToICRS(dir)
	if err != nil {
		log.Fatalf("target position: %v", err)
	}

	target := plan.NewStar("Demo target", targetICRS.RA(), targetICRS.Dec())

	constraint := plan.LimitingMagnitudeConstraint{
		Engine: sky, Passband: johnsonV, Mode: skybrightness.ModeFast, Atmosphere: atmState,
		Conversion: skybrightness.NewSchaeferNELM(),
		Required:   func(plan.Observable) float64 { return targetVMag },
	}

	score, err := plan.ScoreObservableSky(target, obs, site, nil, astro, constraint)
	if err != nil {
		log.Fatalf("score: %v", err)
	}

	fmt.Println("\n======================================================================")
	fmt.Printf("  Target V=%.1f observability score under the moonlit sky: %.3f\n", targetVMag, score)
	fmt.Println("======================================================================")
}
