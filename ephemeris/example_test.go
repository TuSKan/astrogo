package ephemeris_test

import (
	"fmt"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// The task this package exists for: where is a body, at an instant.
//
// [Default] is the zero-configuration provider — SOFA's analytical series for
// the Sun, Moon and the eight major planets. It needs no kernel, no download
// consent and no network, which is what makes it usable in an example that
// runs on every `go test`. For arcsecond work over long spans, swap in a JPL
// SPK-backed provider from ephemeris/jpl; the interface is the same.
func Example() {
	when := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	pos, err := eph.Position(eph.Default(), eph.Mars, when)
	if err != nil {
		panic(err)
	}

	// Geocentric, in AU, on the ICRS axes.
	fmt.Printf("Mars is %.4f AU away\n", pos.Norm())

	// Output:
	// Mars is 2.3127 AU away
}

// ApparentState is the one to reach for when the answer will be pointed at
// something. It applies light-time — the body is seen where it was when the
// light left, not where it is now — which for Mars at opposition distance is
// several minutes of its own motion.
func ExampleApparentState() {
	when := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	geometric, err := eph.Position(eph.Default(), eph.Mars, when)
	if err != nil {
		panic(err)
	}

	apparent, err := eph.ApparentState(eph.Default(), eph.Mars, when)
	if err != nil {
		panic(err)
	}

	// The difference is where Mars moved to while its light was in transit.
	moved := apparent.Pos.Sub(geometric).Norm()

	// 2.31 AU is 1154 s of light time, and Mars and Earth move at up to ~55
	// km/s relative to one another, so tens of thousands of kilometres is the
	// size to expect here rather than a rounding artefact.
	fmt.Printf("light time carries it %.0f km\n", moved*1.495978707e8)

	// Output:
	// light time carries it 63461 km
}
