// Example: Evaluate lunar crescent visibility using all 20 modern criteria.
//
// This demonstrates the plan.CrescentParams evaluation pipeline against
// a representative set of topocentric parameters for a typical
// first-visibility observation scenario.
//
// Reference:
//
//	Al-Jumaili et al., "A Review on Modern Lunar Crescent Visibility
//	Criterion", Malaysian Journal of Science, Vol. 41(3), 2022.
package main

import (
	"fmt"

	"github.com/TuSKan/astrogo/plan"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  Modern Lunar Crescent Visibility Criteria (1910–2021)")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Representative parameters for a marginal crescent sighting.
	p := plan.CrescentParams{
		ArcV: 10.5, // Arc of Vision (degrees)
		ArcL: 12.0, // Elongation (degrees)
		DAZ:  8.0,  // Azimuth difference (degrees)
		MAlt: 5.5,  // Moon altitude (degrees)
		W:    0.5,  // Crescent width (arc minutes)
		LT:   35.0, // Lag time (minutes)
	}

	result := p.EvaluateAll()
	fmt.Println(result.String())
	fmt.Println()

	// Demonstrate individual criterion calls
	fmt.Println("─── Individual Criterion Examples ───")
	fmt.Printf("  Yallop q-value: %.4f → Zone %s\n", result.Yallop.Value, result.Yallop.Code)
	fmt.Printf("  Odeh V-value:   %.4f → %s\n", result.Odeh.Value, result.Odeh.Code)
	fmt.Printf("  Qureshi S-value: %.4f → Zone %s\n", result.Qureshi.Value, result.Qureshi.Code)
}
