// Example: Composite target scoring with configurable weights.
//
// This demonstrates the plan.ScoreObservable merit function that combines
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

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// ── Observatory: São Paulo (-23.5505°, -46.6333°, 760m) ─────────────
	loc, _ := coord.NewGeodetic(angle.Deg(-46.6333), angle.Deg(-23.5505), 760)
	site, _ := plan.NewSite("São Paulo", loc, angle.Zero(), nil)

	// ── Observation epoch: 2026-04-15 at local midnight ──────────────────
	tz, _ := time.LoadLocation("America/Sao_Paulo")
	tm := time.Date(2026, 4, 16, 0, 0, 0, 0, tz)

	// ── Targets (resolved from catalogs) ────────────────────────────────
	ngc := catalog.NewResolver(catalog.OpenNGC)
	simbad := catalog.NewResolver(catalog.SIMBAD)

	type lookup struct {
		name     string
		resolver *catalog.Resolver
	}
	lookups := []lookup{
		{"NGC 5139", ngc},   // Omega Centauri
		{"NGC 3372", ngc},   // Carina Nebula
		{"Sgr A*", simbad},  // Galactic center
		{"Canopus", simbad}, // Alpha Carinae
	}

	targets := make([]plan.Observable, 0, len(lookups))
	for _, l := range lookups {
		t, err := l.resolver.Resolve(l.name)
		if err != nil {
			fmt.Printf("  ⚠ Could not resolve %q: %v\n", l.name, err)
			continue
		}
		targets = append(targets, plan.NewTarget(t, nil))
	}

	// ── Constraints ──────────────────────────────────────────────────────
	constraints := []plan.Constraint{
		plan.Altitude{Threshold: angle.Deg(20)},
		plan.Airmass{Threshold: 2.5},
	}

	// ── Scoring profiles ─────────────────────────────────────────────────
	profiles := []struct {
		name string
		cfg  *plan.ScoreConfig
	}{
		{"Default (balanced)", nil}, // uses DefaultScoreConfig()
		{"Altitude-only", &plan.ScoreConfig{
			AltitudeWeight: 1.0,
			UrgencyWeight:  0.0,
			MoonWeight:     0.0,
		}},
		{"Urgency-heavy", &plan.ScoreConfig{
			AltitudeWeight:     0.2,
			UrgencyWeight:      0.7,
			MoonWeight:         0.1,
			MoonFullPenaltyDeg: 30.0,
		}},
	}

	// ── Header ───────────────────────────────────────────────────────────
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  Composite Target Scoring — São Paulo")
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
			score, err := plan.ScoreObservable(obj, tm, site, profile.cfg, constraints...)
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
	fmt.Printf("  Moon sep:  %.0f%%  (sep/%.0f°, farther from Moon = better)\n", cfg.MoonWeight*100, cfg.MoonFullPenaltyDeg)
	fmt.Println("\n  Score = (w₁·alt + w₂·urgency + w₃·moon) × 90 × priority")
}
