package sgp4_test

import (
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
)

// Reading an element set off a feed: verify the text arrived intact, then read
// the elements out of it.
func ExampleParseTLE() {
	const (
		line1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
		line2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
	)

	if err := sgp4.VerifyTLEChecksums(line1, line2); err != nil {
		fmt.Println("corrupted in transit:", err)

		return
	}

	el, err := sgp4.ParseTLEName("ISS (ZARYA)", line1, line2)
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Printf("%s (%d), epoch %s\n", el.Name, el.NORAD, el.Epoch.Format("2006-01-02 15:04:05"))
	fmt.Printf("inclination %.4f deg, %.8f rev/day\n", el.Inclination.Degrees(), el.MeanMotion)

	// Output:
	// ISS (ZARYA) (25544), epoch 2008-09-20 12:25:40
	// inclination 51.6416 deg, 15.72125391 rev/day
}

// A malformed element set is an error, not a panic and not a plausible orbit.
// The field is named and its columns quoted verbatim, padding included, because
// "malformed TLE" alone tells nobody which of the twenty fields to look at —
// and because in a fixed-width format the padding is part of the evidence.
func ExampleParseTLE_malformed() {
	const (
		line1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
		line2 = "2 25544  XX.XXXX 247.4627 0006703 130.5360 325.0288 15.72125391563537"
	)

	_, err := sgp4.ParseTLE(line1, line2)

	fmt.Println(err)
	fmt.Println("is a malformed TLE:", errors.Is(err, sgp4.ErrMalformedTLE))

	// Output:
	// sgp4: malformed TLE: inclination is " XX.XXXX", which is not a number
	// is a malformed TLE: true
}
