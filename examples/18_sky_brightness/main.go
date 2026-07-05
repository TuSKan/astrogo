// Example: Sky brightness and limiting magnitude under moonlight.
//
// This pairs the plan event API with the skybrightness model:
//   - plan.AstronomicalDawnDusk frames the true-night window,
//   - plan.MoonIllumination / plan.MoonriseMoonset describe the Moon,
//   - a skybrightness.CompositeModel (airglow + zodiacal light, plus scattered
//     moonlight) gives the sky surface brightness toward a pointing, and
//   - skybrightness.VisualLimitingMag turns that into a limiting magnitude.
//
// Finally plan.ScoreObservableSky shows the LimitingMagnitudeConstraint
// demoting a target's observability score under the moonlit sky.
package main

import (
	"context"
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/lightpollution"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// ── Observatory: São Paulo ───────────────────────────────────────────
	tz, _ := time.LoadLocation("America/Sao_Paulo")
	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	site, _ := plan.NewSite("São Paulo", loc, 0, tz)

	provider, err := eph.NewProvider(eph.Planets, "de442")
	if err != nil {
		panic(err)
	}

	// Night of the 2025-03-14 full Moon (local afternoon → next afternoon).
	start := time.Date(2025, 3, 14, 12, 0, 0, 0, tz)
	end := start.AddDate(0, 0, 1)

	// ── Night circumstances via the plan event API ───────────────────────
	dawn, dusk, _ := plan.AstronomicalDawnDusk(start, end, site, provider)
	frac, phase, _ := plan.MoonIllumination(start.Add(12*time.Hour), provider)
	moonrise, moonset, _ := plan.MoonriseMoonset(start, end, site, provider)

	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  Sky Brightness & Limiting Magnitude — São Paulo")
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

	ctx := coord.NewContext(obs, loc, site.Atmosphere())
	moonPos, _ := plan.NewMoon(provider).Position(obs)
	moonAA, _ := ctx.ICRSToAltAz(moonPos)

	fmt.Printf("\n  Observing %s — Moon at altitude %.0f°\n",
		obs.In(tz).Format("Jan 02 15:04 MST"), moonAA.Alt().Degrees())

	// ── Discover the site's artificial light-pollution floor ────────────
	// The natural baseline (airglow + zodiacal light) is physical and assumes
	// nothing. The artificial floor is DISCOVERED from the Falchi 2016 World
	// Atlas via lightpollutionmap.info (set LIGHTPOLLUTIONMAP_KEY). With no key
	// or network we fall back to the natural-only sky.
	components := []skybrightness.Component{
		skybrightness.NewAirglow(),
		skybrightness.NewZodiacalLight(provider),
	}

	if floor, err := lightpollution.New().Floor(context.Background(), -23.5505, -46.6333); err == nil {
		sqm, _ := floor.Radiance(coord.NewAltAz(angle.Deg(90), angle.Deg(0)), nil)
		fmt.Printf("\n  Discovered São Paulo light-pollution floor: %.2f V mag/arcsec² (Falchi 2015)\n",
			float64(sqm.SurfaceBrightnessV()))

		components = append(components, floor)
	} else {
		fmt.Printf("\n  Light-pollution floor unavailable (%v) — using natural sky only.\n", err)
	}

	// ── Sky brightness: floor+natural sky vs the same sky plus moonlight ─
	natural := skybrightness.NewCompositeModel(components...)
	full := skybrightness.NewCompositeModel(append(components, skybrightness.NewMoonlight())...)
	conv := skybrightness.NewVisualLimitingMag()

	fmt.Println("\n── Sky surface brightness (V mag/arcsec², larger = darker) ────")
	fmt.Printf("  %-16s %8s %8s %7s  %s\n", "Pointing", "Natural", "Full", "LimMag", "Equivalent sky")
	report("  toward Moon", coord.NewAltAz(angle.Deg(50), moonAA.Az()), ctx, natural, full, conv)
	report("  away from Moon", coord.NewAltAz(angle.Deg(50), moonAA.Az().Add(angle.Deg(180))), ctx, natural, full, conv)

	// ── Constraint: moonlight demotes a target's observability score ─────
	lst, _ := site.LocalSiderealTime(obs)
	star := plan.NewStar("zenith star (V=5.5)", lst, site.Latitude())
	required := func(plan.Observable) float64 { return 5.5 }

	base, _ := plan.ScoreObservable(star, obs, site, nil, ctx)
	naturalScore, _ := plan.ScoreObservableSky(star, obs, site, nil, ctx,
		plan.LimitingMagnitudeConstraint{Model: natural, Conversion: conv, Required: required})
	moonScore, _ := plan.ScoreObservableSky(star, obs, site, nil, ctx,
		plan.LimitingMagnitudeConstraint{Model: full, Conversion: conv, Required: required})

	fmt.Println("\n── Observability score for a V=5.5 target near zenith ────────")
	fmt.Printf("  Base score:            %6.1f\n", base)
	fmt.Printf("  Natural sky score:     %6.1f\n", naturalScore)
	fmt.Printf("  Moonlit sky score:     %6.1f\n", moonScore)
}

// report prints the floor-only and full-model sky brightness plus the limiting
// magnitude toward a pointing.
func report(name string, aa coord.AltAz, ctx *coord.Context, floor, full skybrightness.Model, conv skybrightness.LimitingMagModel) {
	naturalSB, _ := floor.SurfaceBrightness(aa, ctx)
	fullSB, _ := full.SurfaceBrightness(aa, ctx)
	airmass, _ := atmosphere.Airmass(aa.Alt())
	limMag, _ := conv.LimitingMagnitude(fullSB, airmass)

	class, sky := skybrightness.BortleClass(fullSB)

	fmt.Printf("  %-16s %8.2f %8.2f %7.2f  Bortle %d (%s)\n",
		name, float64(naturalSB), float64(fullSB), limMag, class, sky)
}

// printEvent prints a rise/set/twilight event in local time, or "—" if absent.
func printEvent(label string, e *plan.Event, tz *time.Location) {
	if e == nil {
		fmt.Printf("%s: —\n", label)

		return
	}

	fmt.Printf("%s: %s\n", label, e.Time.In(tz).Format("Jan 02 15:04 MST"))
}
