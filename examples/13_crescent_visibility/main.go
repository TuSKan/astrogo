// Example: Evaluate lunar crescent visibility from computed ephemeris.
//
// This finds the next New Moon, then evaluates crescent visibility on the
// following evening from São Paulo using real Sun/Moon positions computed
// via NewCrescentParams. All 20 modern criteria (1910–2021) are evaluated.
//
// Reference:
//
//	Al-Jumaili et al., "A Review on Modern Lunar Crescent Visibility
//	Criterion", Malaysian Journal of Science, Vol. 41(3), 2022.
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  Lunar Crescent Visibility — São Paulo")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// ── Observer: São Paulo, Brazil ──────────────────────────────────────
	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	site, _ := plan.NewSite("São Paulo", loc, 0, nil)

	eph := ephemeris.Default()

	// ── Find the next New Moon ──────────────────────────────────────────
	now := time.NowUTC()
	newMoon, err := plan.NextNewMoon(now, eph)
	if err != nil {
		log.Fatalf("NextNewMoon: %v", err)
	}
	fmt.Printf("  Next New Moon: %s\n\n", newMoon.Time)

	// ── Find sunset on the evening after the New Moon ───────────────────
	// The crescent is typically first visible on the evening following
	// the astronomical New Moon (conjunction).
	evening := newMoon.Time
	nextDay := evening.AddDays(1)

	_, sunset, err := plan.SunriseSunset(evening, nextDay, site, eph)
	if err != nil {
		log.Fatalf("SunriseSunset: %v", err)
	}
	fmt.Printf("  Sunset (São Paulo): %s\n\n", sunset.Time)

	// ── Compute crescent parameters ~20 min after sunset ────────────────
	// Best-practice observation window: 15–30 min after sunset, when the
	// sky is dark enough to see a thin crescent but the Moon is still
	// above the horizon.
	obsTime := sunset.Time.Add(20 * time.Minute)
	fmt.Printf("  Observation time:   %s (sunset + 20 min)\n\n", obsTime)

	params, err := plan.NewCrescentParams(obsTime, loc, eph)
	if err != nil {
		log.Fatalf("NewCrescentParams: %v", err)
	}

	// ── Evaluate all 20 criteria ────────────────────────────────────────
	result := params.EvaluateAll()
	fmt.Println(result.String())
	fmt.Println()

	// ── Summary ─────────────────────────────────────────────────────────
	fmt.Println("─── Multi-Zone Classification ──────────────────────────────")
	fmt.Printf("  Yallop (1998):  Zone %s — %s (q=%.4f)\n",
		result.Yallop.Code, result.Yallop.Label, result.Yallop.Value)
	fmt.Printf("  Odeh (2004):    %s — %s (V=%.4f)\n",
		result.Odeh.Code, result.Odeh.Label, result.Odeh.Value)
	fmt.Printf("  Qureshi (2010): Zone %s — %s (S=%.4f)\n",
		result.Qureshi.Code, result.Qureshi.Label, result.Qureshi.Value)
}
