// Example: Sky Brightness V2 — spectral sky-radiance evaluation.
//
// This is the mandated end-to-end demonstration for Phase 1
// (docs/skybrightness.md §14): site + time + atmosphere + direction ->
// spectral component decomposition -> passband brightness -> uncertainty
// -> limiting magnitude -> full provenance.
//
// Phase 1 honesty note, stated here and in the printed output: this
// package's ONLY real physics today is the Legacy* fast models
// (natural.LegacyAirglow, natural.LegacyMoonlight — the exact v1
// Krisciunas & Schaefer 1991 physics, re-implemented against the new
// spectral API) plus an analytic Rayleigh-only transmission model
// (atmos.RayleighOnly). Real spectral zodiacal light, starlight, diffuse
// galactic light, twilight, and artificial-emission propagation are
// Phase 2-5 scope — see docs/skybrightness.md's staged implementation
// plan. What this example proves is the ENGINE, not yet the full sky.
//
// This pairs the plan event API with the new skybrightness.Engine:
//   - plan.AstronomicalDawnDusk frames the true-night window,
//   - plan.MoonIllumination / plan.MoonriseMoonset describe the Moon,
//   - a skybrightness.CompositeEngine (LegacyAirglow + LegacyMoonlight,
//     ModeLegacy) gives the sky's spectral radiance toward a pointing,
//     reduced to a V-band magnitude through natural.LegacyJohnsonV() —
//     the only passband whose output is physically meaningful in Phase 1
//     (see legacy_units.go's doc comment for why),
//   - skybrightness.LegacySchaeferNELM turns that into a limiting
//     magnitude, and
//   - plan.ScoreObservableSky shows LimitingMagnitudeConstraint demoting a
//     target's observability score under the moonlit sky.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
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
	astro := coord.NewContext(obs, site.Location(), site.Atmosphere())

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

	// ── Atmosphere: explicit, user-supplied, immutable ───────────────────
	atmState, err := skybrightness.NewAtmosphereBuilder().
		Surface(1013.25, 288.15).
		Clear(). // no cloud layers — Phase 1 has no cloud optics yet regardless
		SurfaceAlbedo(skybrightness.UniformAlbedo(0.15)).
		Source(skybrightness.SourceRef{Name: "user-supplied", Fidelity: skybrightness.FidelityMeasured}).
		Build()
	if err != nil {
		log.Fatalf("atmosphere: %v", err)
	}

	// ── Engine: components assembled by the application, not by plan or
	// by core skybrightness (docs/skybrightness.md §4) ───────────────────
	sky, err := skybrightness.NewCompositeEngine(skybrightness.CompositeConfig{
		Name: skybrightness.AlgorithmRef{Name: "examples/18_sky_brightness", Version: "phase1"},
		Components: []skybrightness.Component{
			natural.NewLegacyAirglow(),
			natural.NewLegacyMoonlight(natural.WithLegacyMoonProvider(provider)),
		},
		Transmission: atmos.NewRayleighOnly(),
		Mode:         skybrightness.ModeLegacy,
	})
	if err != nil {
		log.Fatalf("build engine: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()
	johnsonV := natural.LegacyJohnsonV()

	dir := coord.NewAltAz(angle.Deg(pointingAltitude), angle.Deg(120))

	res, err := sky.Evaluate(ctx, skybrightness.Request{
		Astro: astro, Directions: []coord.AltAz{dir}, Grid: grid,
		Passbands: []*skybrightness.Passband{johnsonV},
		Mode:      skybrightness.ModeLegacy, Atmosphere: atmState,
		Selection: skybrightness.ComponentSelection{Materialize: true},
		Options: skybrightness.EvaluationOptions{
			ComputeTransmission: true,
			Derived:             skybrightness.DerivePassbands | skybrightness.DeriveLimitingMag,
			Uncertainty:         skybrightness.UncLinearized,
			Fallback:            skybrightness.FallbackForbidden,
			LimitingMag:         skybrightness.NewLegacySchaeferNELM(),
		},
	})
	if err != nil {
		log.Fatalf("evaluate: %v", err)
	}

	fmt.Println("\n── Component decomposition (Garstang nanolambert convention, Phase 1) ──")
	fmt.Println("  Meaningful only through LegacyJohnsonV — see the doc comment above.")

	res.Components.Each(func(id skybrightness.ComponentID, f skybrightness.SpectralField, rep skybrightness.ComponentReport) bool {
		r, ierr := skybrightness.IntegrateRadiance(grid, f.Row(0), johnsonV)
		if ierr != nil {
			fmt.Printf("  %-16s (integration error: %v)\n", id, ierr)
			return true
		}

		fmt.Printf("  %-16s %12.4g  (+/- %.0f%%)  %v\n",
			id, float64(r), 100*rep.Uncertainty.RelSigma, rep.Quality.Strings())

		return true
	})

	fmt.Println("\n── Passband brightness ──────────────────────────────────────────────")

	for _, pb := range res.Derived.Passbands {
		fmt.Printf("  %-16s Vega = %.2f mag/arcsec^2\n", pb.Passband, pb.Vega[0])
	}

	fmt.Printf("\n  Atmospheric transmission (Rayleigh-only, Phase 1) at grid ends:\n")

	if len(res.Transmission) >= grid.Len() {
		fmt.Printf("    %.0fnm: %.3f    %.0fnm: %.3f\n",
			float64(grid.At(0)), res.Transmission[0],
			float64(grid.At(grid.Len()-1)), res.Transmission[grid.Len()-1])
	}

	if len(res.Derived.LimitingMagnitude) > 0 {
		fmt.Printf("\n  Limiting magnitude (Schaefer 1990, LegacyJohnsonV): %.2f\n", res.Derived.LimitingMagnitude[0])
	}

	fmt.Printf("  Quality flags: %v\n", res.Quality.Strings())

	prov, err := json.MarshalIndent(res.Provenance, "", "  ")
	if err != nil {
		log.Fatalf("marshal provenance: %v", err)
	}

	fmt.Printf("\n── Provenance ────────────────────────────────────────────────────────\n")
	fmt.Printf("  digest: %x\n", res.Provenance.Digest())
	fmt.Printf("%s\n", prov)

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
		Engine: sky, Passband: johnsonV, Mode: skybrightness.ModeLegacy, Atmosphere: atmState,
		Conversion: skybrightness.NewLegacySchaeferNELM(),
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
