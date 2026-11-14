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
	"github.com/TuSKan/astrogo/examples/internal/show"
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
	// A target failing any of them scores zero, whatever the weights.
	altitude := plan.Altitude{Threshold: angle.Deg(20)}
	constraints := []plan.Constraint{altitude, plan.Airmass{Threshold: 2.5}}

	// ── Scoring profiles ─────────────────────────────────────────────────
	profiles := []struct {
		cfg  plan.ScoreConfig
		name string
	}{
		{plan.ScoreConfig{}, "Default"}, // the zero value means DefaultScoreConfig()
		{plan.ScoreConfig{
			AltitudeWeight: 1.0,
			UrgencyWeight:  0.0,
			MoonWeight:     0.0,
		}, "Altitude only"},
		{plan.ScoreConfig{
			AltitudeWeight:  0.2,
			UrgencyWeight:   0.7,
			MoonWeight:      0.1,
			MoonFullPenalty: angle.Deg(30),
		}, "Urgency heavy"},
	}

	show.Title("Composite target scoring", site.Name()+", "+tm.Format("2006-01-02 15:04 MST"))

	cols := make([]show.Column, 0, 2+len(profiles))
	cols = append(cols, show.Left("Target"), show.Right("Altitude"))

	for _, p := range profiles {
		cols = append(cols, show.Right(p.name))
	}

	tab := show.NewTable(cols...)

	for _, obj := range targets {
		alt, err := altitude.Check(obj, tm, site)
		if err != nil {
			log.Fatalf("%s altitude: %v", obj.Name(), err)
		}

		row := make([]string, 0, 2+len(profiles))
		row = append(row, obj.Name(), fmt.Sprintf("%.1f°", alt.Value))

		for _, p := range profiles {
			score, err := plan.Scorer{Site: site, Config: p.cfg, Constraints: constraints}.Score(obj, tm)
			if err != nil {
				log.Fatalf("%s %s score: %v", obj.Name(), p.name, err)
			}

			row = append(row, fmt.Sprintf("%.1f", score))
		}

		tab.Row(row...)
	}

	fmt.Println()
	tab.Print()
	fmt.Println()
	show.Note("Canopus scores zero under every profile: it is below the",
		"20° altitude limit, and a target that fails a constraint is out.")

	// ── Explain the default weights ──────────────────────────────────────
	cfg := plan.DefaultScoreConfig()

	show.Section("The default weights")
	show.Fields(
		show.KV("Altitude", "%.0f%%  altitude/90°: higher is lower airmass", cfg.AltitudeWeight*100),
		show.KV("Urgency", "%.0f%%  1/hours until set: about to set is urgent", cfg.UrgencyWeight*100),
		show.KV("Moon", "%.0f%%  separation/%.0f°: farther from the Moon is better",
			cfg.MoonWeight*100, cfg.MoonFullPenalty.Degrees()),
	)
	fmt.Println()
	show.Note("Score = (w₁·altitude + w₂·urgency + w₃·moon) × 90 × priority")
}
