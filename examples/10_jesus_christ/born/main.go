// Example: Star of Bethlehem — Jupiter-Saturn triple conjunction of 7 BC
// and the Jupiter-Venus conjunction of 2 BC.
//
// Requires DE441 for deep historical epoch coverage (13200 BC – AD 17191).
// The kernel (~3 GB) is auto-downloaded on first run.
package main

import (
	"fmt"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	prov, err := eph.NewProvider(eph.Planets, "de441_part-1")
	if err != nil {
		panic(err)
	}
	defer prov.Close()

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
	s1 := time.Date(-6, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	e1 := time.Date(-5, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	conjs, err := plan.Conjunctions(s1, e1, jupiter, saturn)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else if len(conjs) == 0 {
		fmt.Println("  No conjunctions found")
	} else {
		for i, c := range conjs {
			// Compute the angular separation at conjunction time
			jPos, _ := jupiter.Position(c.Time)
			sPos, _ := saturn.Position(c.Time)
			sep := coord.Separation(jPos, sPos)

			// Compute solar elongation to determine sky position
			sunPos, _ := sun.Position(c.Time)
			elong := coord.Separation(jPos, sunPos)
			sky := skyPosition(elong.Degrees())

			fmt.Printf("  #%d  %s  (sep: %.2f°, elong: %.0f° — %s)\n",
				i+1, c.Time.FormatJulian("2006-01-02 15:04 MST"),
				sep.Degrees(), elong.Degrees(), sky)
		}
	}

	// ── Ecliptic longitude conjunctions (classical definition) ──
	fmt.Println("\n  Ecliptic longitude conjunctions (classical definition, Δλ=0):")
	eclConjs, err := plan.ConjunctionsEcliptic(s1, e1, jupiter, saturn)
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
	appulses, err := plan.Appulses(s1, e1, jupiter, saturn)
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
	//  and check when it crosses Regulus's longitude at RA ≈ 10h08m)
	// ──────────────────────────────────────────────────────────────────
	fmt.Println("\n▸ Jupiter–Venus Conjunction, June 17, 2 BC:")
	// Astronomical year: 2 BC = year −1
	s2 := time.Date(-1, time.June, 1, 0, 0, 0, 0, time.LocationUTC)
	e2 := time.Date(-1, time.July, 15, 0, 0, 0, 0, time.LocationUTC)

	conjs2, err := plan.Conjunctions(s2, e2, jupiter, venus)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else if len(conjs2) == 0 {
		fmt.Println("  No conjunctions found")
	} else {
		for _, c := range conjs2 {
			jPos, _ := jupiter.Position(c.Time)
			vPos, _ := venus.Position(c.Time)
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
	s3 := time.Date(-2, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	e3 := time.Date(0, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	conjs3, err := plan.Conjunctions(s3, e3, jupiter, saturn)
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
//   - Low elongation early in the year → morning sky (approaching conjunction)
//   - Low elongation late in the year → evening sky (receding from conjunction)
//   - High elongation → all night (near opposition, retrograde)
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
