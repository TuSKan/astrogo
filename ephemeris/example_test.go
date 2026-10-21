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
//
// A provider locates each body about the centre it computes it from; [At]
// composes those into a state about any centre at one instant, here the
// Earth's.
func Example() {
	when := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	mars, err := eph.At(eph.Default(), when).Geometric(eph.MarsBarycenter, eph.Earth)
	if err != nil {
		panic(err)
	}

	// Geocentric, in AU, on the ICRS axes.
	fmt.Printf("Mars is %.4f AU away\n", mars.Pos.Norm())

	// Output:
	// Mars is 2.3127 AU away
}

// The apparent place is the one to reach for when the answer will be pointed
// at something. It applies light time — the body is seen where it was when the
// light left, not where it is now — then the Sun's deflection of that light
// and the aberration of the Earth's motion, in SOFA's order.
func ExampleInstant_Apparent() {
	when := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)
	in := eph.At(eph.Default(), when)

	geometric, err := in.Geometric(eph.MarsBarycenter, eph.Earth)
	if err != nil {
		panic(err)
	}

	apparent, err := in.Apparent(eph.MarsBarycenter, eph.Geocenter())
	if err != nil {
		panic(err)
	}

	// The light left Mars about 19 minutes earlier, 2.31 AU away, and in that
	// time Mars moved; the aberration of the Earth's 30 km/s turns the
	// direction by up to 20.5″ more. Tens of thousands of kilometers, at that
	// distance, is the size to expect rather than a rounding artefact.
	moved := apparent.Pos.Sub(geometric.Pos).Norm()
	fmt.Printf("geometric to apparent is %.0f km\n", moved*1.495978707e8)

	// Output:
	// geometric to apparent is 63684 km
}
