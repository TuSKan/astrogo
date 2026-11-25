// Example: Star of Bethlehem — Jupiter-Saturn triple conjunction of 7 BC
// and the Jupiter-Venus conjunction of 2 BC.
//
// Requires DE441 for deep historical epoch coverage (13200 BC – AD 17191).
// The kernel (~3 GB) downloads on first run — see README "Data downloads
// & offline usage".
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/plan"
)

func main() {
	// de441 parts are multi-GB each and download on first run, which is the
	// point of this example. naif0012.tls (leap seconds) is ~5 KB. Earth
	// orientation loads on first need, a download the first time. No bulletin
	// covers the first century, though (it begins in 1973), so astrogo uses
	// zero EOP here and says so once. What sets the accuracy is Delta T, the
	// drift of the Earth's rotation, from the Espenak & Meeus (2006) model.
	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de441_part-1")
	if err != nil {
		panic(err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	jupiter := plan.NewJupiter(prov)
	saturn := plan.NewSaturn(prov)
	venus := plan.NewVenus(prov)
	sun := plan.NewSun(prov)

	fmt.Println("══════════════════════════════════════════════════════════")
	fmt.Println("  Star of Bethlehem Candidates — astrogo + JPL DE441")
	fmt.Println("══════════════════════════════════════════════════════════")

	// ──────────────────────────────────────────────────────────────────
	// Candidate 1: Jupiter–Saturn triple conjunction, 7 BC
	// ──────────────────────────────────────────────────────────────────
	fmt.Println("\n▸ Jupiter–Saturn Conjunctions, 7 BC (astronomical year −6):")

	s1 := epoch.DateJulianCal(-6, 1, 1, 0, 0, 0)
	e1 := epoch.DateJulianCal(-5, 1, 1, 0, 0, 0)

	conjs, err := plan.Conjunctions(jupiter, saturn, plan.Window{Start: s1, End: e1})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else if len(conjs) == 0 {
		fmt.Println("  No conjunctions found")
	} else {
		for i, c := range conjs {
			// Compute the angular separation at conjunction time
			jPos, err := jupiter.Position(c.Time)
			if err != nil {
				log.Fatalf("jPos: %v", err)
			}

			sPos, err := saturn.Position(c.Time)
			if err != nil {
				log.Fatalf("sPos: %v", err)
			}

			sep := coord.Separation(jPos, sPos)

			// Compute solar elongation to determine sky position
			sunPos, err := sun.Position(c.Time)
			if err != nil {
				log.Fatalf("sunPos: %v", err)
			}

			elong := coord.Separation(jPos, sunPos)
			sky := skyPosition(elong.Degrees())

			fmt.Printf("  #%d  %s  (sep: %.2f°, elong: %.0f° — %s)\n",
				i+1, c.Time.FormatJulian("2006-01-02 15:04 MST"),
				sep.Degrees(), elong.Degrees(), sky)
		}
	}

	// ── Ecliptic longitude conjunctions (classical definition) ──
	fmt.Println("\n  Ecliptic longitude conjunctions (classical definition, Δλ=0):")

	eclConjs, err := plan.ConjunctionsEcliptic(jupiter, saturn, plan.Window{Start: s1, End: e1})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else {
		for i, c := range eclConjs {
			fmt.Printf("  #%d  %s (Δλ=0)\n",
				i+1, c.Time.FormatJulian("2006-01-02 15:04 MST"))
		}
	}

	// ── Appulses (minimum angular separation) ──
	fmt.Println("\n  Appulses (closest visual approach):")

	appulses, err := plan.Appulses(jupiter, saturn, plan.Window{Start: s1, End: e1})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else {
		for i, a := range appulses {
			fmt.Printf("  #%d  %s  (min sep: %.2f°)\n",
				i+1, a.Time.FormatJulian("2006-01-02 15:04 MST"), a.Value)
		}
	}

	// ──────────────────────────────────────────────────────────────────
	// Candidate 2: Jupiter–Regulus triple conjunction, 3–2 BC
	// (Regulus is a fixed star — we compute Jupiter's ecliptic longitude
	// ──────────────────────────────────────────────────────────────────
	fmt.Println("\n▸ Jupiter–Venus Conjunction, June 17, 2 BC:")
	// Astronomical year: 2 BC = year −1
	s2 := epoch.DateJulianCal(-1, 6, 1, 0, 0, 0)
	e2 := epoch.DateJulianCal(-1, 7, 15, 0, 0, 0)

	conjs2, err := plan.Conjunctions(jupiter, venus, plan.Window{Start: s2, End: e2})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else if len(conjs2) == 0 {
		fmt.Println("  No conjunctions found")
	} else {
		for _, c := range conjs2 {
			jPos, err := jupiter.Position(c.Time)
			if err != nil {
				log.Fatalf("jPos: %v", err)
			}

			vPos, err := venus.Position(c.Time)
			if err != nil {
				log.Fatalf("vPos: %v", err)
			}

			sep := coord.Separation(jPos, vPos)
			fmt.Printf("  %s  (sep: %.4f° ≈ %.1f arcmin)\n",
				c.Time.FormatJulian("2006-01-02 15:04 MST"),
				sep.Degrees(), sep.Degrees()*60)
		}
	}

	// ──────────────────────────────────────────────────────────────────
	// Extra: Jupiter–Saturn conjunctions in 3–2 BC for comparison
	// ──────────────────────────────────────────────────────────────────
	fmt.Println("\n▸ Jupiter–Saturn geometry, 3–2 BC (for comparison):")

	s3 := epoch.DateJulianCal(-2, 1, 1, 0, 0, 0)
	e3 := epoch.DateJulianCal(0, 1, 1, 0, 0, 0)

	conjs3, err := plan.Conjunctions(jupiter, saturn, plan.Window{Start: s3, End: e3})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else if len(conjs3) == 0 {
		fmt.Println("  No Jupiter–Saturn conjunction in 3–2 BC (expected)")
	} else {
		for _, c := range conjs3 {
			fmt.Printf("  %s\n", c.Time.FormatJulian("2006-01-02 15:04 MST"))
		}
	}

	fmt.Println()
}

// skyPosition classifies the sky visibility based on solar elongation.
// Elongation < 90° → object is relatively close to the Sun (morning or evening).
// Elongation > 120° → near opposition, visible most of the night.
// We use RA difference sign to distinguish morning (west of Sun = rises before)
// from evening (east of Sun = sets after), but since Separation() returns an
// unsigned angle, we use elongation magnitude as a proxy:
func skyPosition(elongDeg float64) string {
	if elongDeg > 120 {
		return "All night (near opposition)"
	}

	if elongDeg > 90 {
		return "Most of night"
	}

	if elongDeg > 45 {
		return "Evening or morning sky"
	}

	return "Near Sun (difficult)"
}
