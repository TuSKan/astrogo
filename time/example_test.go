package time_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/time"
)

// Every example here uses a fixed epoch rather than the clock. That is the
// advice the package doc gives for reproducible work, and it is also what lets
// these carry an `// Output:` comment — which is what makes them run, and diff,
// on every `go test`.

// The task this package exists for: hold an instant and know which scale it is
// on, so that arithmetic between two instants cannot silently mix them.
func Example() {
	// 2026 March 20, 09:01:30 UTC — near the equinox, on no particular boundary.
	t := time.Date(2026, time.March, 20, 9, 1, 30, 0, time.LocationUTC)

	fmt.Println("scale:", t.Scale())
	fmt.Printf("JD:    %.5f\n", t.JD())
	fmt.Printf("MJD:   %.5f\n", t.MJD())

	// Output:
	// scale: UTC
	// JD:    2461119.87604
	// MJD:   61119.37604
}

// The scales differ in what they call the same instant, not in which instant
// they name. That distinction is what this package is built around, and it is
// worth seeing spelled out.
func ExampleTime_TAI() {
	utc := time.Date(2026, time.March, 20, 9, 1, 30, 0, time.LocationUTC)

	tai := utc.TAI()
	tt := utc.TT()

	// One moment, so Sub is zero across every pair: Sub converts to a common
	// scale before subtracting, which is the whole point of the type.
	fmt.Printf("tai.Sub(utc) = %v\n", tai.Sub(utc))

	// The LABELS differ, and that is what the leap-second count buys. Compare
	// Julian Dates rather than instants to see it: TAI reads 37 s ahead of UTC
	// (the leap seconds accumulated since 1972), and TT a further 32.184 s
	// ahead of TAI, exact by definition.
	const secondsPerDay = 86400

	fmt.Printf("TAI label - UTC label = %.3f s\n", (tai.JD()-utc.JD())*secondsPerDay)
	fmt.Printf("TT  label - TAI label = %.3f s\n", (tt.JD()-tai.JD())*secondsPerDay)

	// Output:
	// tai.Sub(utc) = 0s
	// TAI label - UTC label = 37.000 s
	// TT  label - TAI label = 32.184 s
}

// Comparison and arithmetic convert to a common scale first, so two instants on
// different scales compare by the moment they name rather than by the number
// they carry. Getting that wrong is worth 69.184 seconds — 530 km of ISS track.
func ExampleTime_Sub() {
	utc := time.Date(2026, time.March, 20, 9, 1, 30, 0, time.LocationUTC)
	tt := utc.TT()

	fmt.Printf("equal across scales: %v\n", tt.Equal(utc))

	// An hour later, still measured across scales.
	later := utc.Add(time.Hour)
	fmt.Printf("later - tt = %v\n", later.Sub(tt))

	// Output:
	// equal across scales: true
	// later - tt = 1h0m0s
}
