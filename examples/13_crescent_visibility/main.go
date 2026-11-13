// Example: Evaluate lunar crescent visibility from computed ephemeris.
//
// This finds the next New Moon, then evaluates crescent visibility on the
// following evening from Quinta Calixto with CrescentVisibility, which finds
// the sunset and moonset and evaluates 18 published criteria (1910–2021),
// each in the convention it was defined in.
//
// Reference:
//
//	Al-Jumaili et al., "A Review on Modern Lunar Crescent Visibility
//	Criterion", Malaysian Journal of Science, Vol. 41(3), 2022.
package main

import (
	"fmt"
	"log"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  Lunar Crescent Visibility — Quinta Calixto")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move the Moon's and Sun's
	// altitudes by up to 13.5 arcsec, far below the tenths of a degree the
	// criteria work in. For full accuracy, blank-import remote/eop and
	// consent to the IERS bulletin, as 05_resolve_name does.
	time.RegisterModel(time.ZeroModel{})

	// ── Observer: Quinta Calixto, Brazil ─────────────────────────────────
	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	prov := eph.Default()

	// ── Find the first New Moon of December 2026 ────────────────────────
	// A fixed start keeps the output the same on every run; start from
	// time.NowUTC() to look for the next one from today. Times print in
	// the zone of the start, here the site's.
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	from := time.Date(2026, 12, 1, 0, 0, 0, 0, tz)

	newMoon, err := plan.NextNewMoon(from, prov)
	if err != nil {
		log.Fatalf("NextNewMoon: %v", err)
	}

	fmt.Printf("  New Moon: %s\n\n", newMoon.Time)

	// ── Evaluate the first evening after the New Moon ───────────────────
	// The crescent is typically first visible on the evening following
	// the astronomical New Moon (conjunction). CrescentVisibility takes the
	// first sunset after the instant it is given, the moonset after it, and
	// Yallop's best time between them, 4/9 of the lag after sunset, when
	// the sky is dark enough to see a thin crescent but the Moon is still
	// above the horizon.
	result, err := plan.CrescentVisibility(newMoon.Time, site, prov)
	if err != nil {
		log.Fatalf("CrescentVisibility: %v", err)
	}

	fmt.Printf("  Sunset (Quinta Calixto): %s\n", result.Sunset)
	fmt.Printf("  Moonset:                 %s\n", result.Moonset)
	fmt.Printf("  Best time:               %s\n\n", result.BestTime)

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
