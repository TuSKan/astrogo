// Example: Composite target scoring with configurable weights.
//
// This demonstrates the plan.Scorer merit function that combines
// altitude, urgency (time until set), and Moon separation into a single
// score for observation prioritization.
//
// Three ScoreConfig presets are compared:
//   - Default (balanced)
//   - Altitude-only (classic airmass-optimized)
//   - Urgency-heavy (time-critical survey mode)
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move an alt/az position by up
	// to 13.5 arcsec, which changes no score at the precision printed. For
	// full accuracy, blank-import remote/eop and consent to the IERS
	// bulletin, as 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

	// ── Observatory: Quinta Calixto (-22.528478°, -46.473002°, 835.05m) ──
	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	// ── Observation epoch: 2026-04-16 at local midnight ──────────────────
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	tm := time.Date(2026, 4, 16, 0, 0, 0, 0, tz)

	// ── Targets, at their J2000 positions from SIMBAD ───────────────────
	// 05_resolve_name shows how to look a name up in a catalog instead.
	targets := []plan.Observable{
		plan.NewDeepSkyObject("Omega Centauri", angle.Deg(201.697000), angle.Deg(-47.479472)),
		plan.NewDeepSkyObject("Carina Nebula", angle.Deg(161.259292), angle.Deg(-59.699944)),
		plan.NewDeepSkyObject("Sgr A*", angle.Deg(266.416817), angle.Deg(-29.007825)),
		plan.NewStar("Canopus", angle.Deg(95.987958), angle.Deg(-52.695661),
			plan.WithProperMotion(angle.Arcsec(0.01993), angle.Arcsec(0.02324)),
			plan.WithParallax(angle.Arcsec(0.01055)),
			plan.WithStarMagnitude(-0.74)),
	}

	// ── Constraints ──────────────────────────────────────────────────────
	constraints := []plan.Constraint{
		plan.Altitude{Threshold: angle.Deg(20)},
		plan.Airmass{Threshold: 2.5},
	}

	// ── Scoring profiles ─────────────────────────────────────────────────
	profiles := []struct {
		cfg  plan.ScoreConfig
		name string
	}{
		{plan.ScoreConfig{}, "Default (balanced)"}, // the zero value means DefaultScoreConfig()
		{plan.ScoreConfig{
			AltitudeWeight: 1.0,
			UrgencyWeight:  0.0,
			MoonWeight:     0.0,
		}, "Altitude-only"},
		{plan.ScoreConfig{
			AltitudeWeight:  0.2,
			UrgencyWeight:   0.7,
			MoonWeight:      0.1,
			MoonFullPenalty: angle.Deg(30),
		}, "Urgency-heavy"},
	}

	// ── Header ───────────────────────────────────────────────────────────
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  Composite Target Scoring — Quinta Calixto")
	fmt.Printf("  Epoch: %s\n", tm)
	fmt.Println("══════════════════════════════════════════════════════════════")

	for _, profile := range profiles {
		fmt.Printf("\n── %s ", profile.name)
		fmt.Println("──────────────────────────────────────")

		// Score each target
		type result struct {
			name  string
			score float64
		}

		var results []result

		for _, obj := range targets {
			score, err := plan.Scorer{
				Site:        site,
				Config:      profile.cfg,
				Constraints: constraints,
			}.Score(obj, tm)
			if err != nil {
				fmt.Printf("  %-14s  error: %v\n", obj.Name(), err)
				continue
			}

			results = append(results, result{obj.Name(), score})
		}

		// Print ranked
		fmt.Printf("  %-14s  %8s\n", "Target", "Score")
		fmt.Printf("  %-14s  %8s\n", "──────────────", "────────")

		for _, r := range results {
			marker := " "
			if r.score == 0 {
				marker = "✗"
			}

			fmt.Printf("  %-14s  %8.1f  %s\n", r.name, r.score, marker)
		}
	}

	// ── Explain the default weights ──────────────────────────────────────
	fmt.Println("\n── Weight Breakdown ──────────────────────────────────────────")

	cfg := plan.DefaultScoreConfig()
	fmt.Printf("  Altitude:  %.0f%%  (alt/90°, lower airmass = better)\n", cfg.AltitudeWeight*100)
	fmt.Printf("  Urgency:   %.0f%%  (1/hours_until_set, about-to-set = urgent)\n", cfg.UrgencyWeight*100)
	fmt.Printf("  Moon sep:  %.0f%%  (sep/%.0f°, farther from Moon = better)\n", cfg.MoonWeight*100, cfg.MoonFullPenalty.Degrees())
	fmt.Println("\n  Score = (w₁·alt + w₂·urgency + w₃·moon) × 90 × priority")
}
