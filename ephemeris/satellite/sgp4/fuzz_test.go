package sgp4_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
)

// FuzzParseTLE asserts the property this package was written to have: no input
// makes it do anything except return.
//
// The bar is not "parses correctly" — correctness is [TestParseTLEReadsEveryField]'s
// job. It is that a library handed hostile bytes never panics, never calls
// [log.Fatal], and never exits the process its caller is running. The
// implementation astrogo used before this one could do all three, and a fuzzer
// found the panic in about two seconds.
//
// An element set arrives from CelesTrak, Space-Track, or a file a user picked,
// so "hostile" here is not hypothetical.
func FuzzParseTLE(f *testing.F) {
	f.Add(issLine1, issLine2)
	f.Add("", "")
	f.Add("1", "2")

	// The crasher the previous implementation took: a day of year past the end
	// of its year, in an element set that is otherwise perfect.
	f.Add(
		"1 25544U 98067A   08400.51782528 -.00002182  00000-0 -11606-4 0  2927",
		issLine2,
	)

	// Every field simultaneously blank — each parser has to decide between a
	// zero and an error without reading off the end.
	f.Add(
		"1                                                                    ",
		"2                                                                    ",
	)

	// Non-ASCII in fixed columns, where byte indexing and rune indexing differ.
	f.Add(
		"1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  29é7",
		issLine2,
	)

	f.Fuzz(func(t *testing.T, line1, line2 string) {
		el, err := sgp4.ParseTLE(line1, line2)
		if err != nil {
			// The one thing a failed parse must not do is return something
			// usable alongside the error.
			if el != (sgp4.Elements{}) {
				t.Errorf("ParseTLE returned both an error and a populated element set")
			}

			return
		}

		// A successful parse is a promise that Validate passed, so the values
		// are inside their stated ranges and nothing downstream has to re-check.
		if verr := el.Validate(); verr != nil {
			t.Errorf("ParseTLE returned an element set that fails its own Validate: %v", verr)
		}

		for _, f := range []struct {
			name string
			v    float64
		}{
			{"MeanMotion", el.MeanMotion},
			{"Eccentricity", el.Eccentricity},
			{"Inclination", el.Inclination.Radians()},
			{"RAAN", el.RAAN.Radians()},
			{"ArgPerigee", el.ArgPerigee.Radians()},
			{"MeanAnomaly", el.MeanAnomaly.Radians()},
			{"BStar", el.BStar},
			{"MeanMotionDot", el.MeanMotionDot},
			{"MeanMotionDDot", el.MeanMotionDDot},
		} {
			if math.IsNaN(f.v) || math.IsInf(f.v, 0) {
				t.Errorf("ParseTLE succeeded with %s = %v", f.name, f.v)
			}
		}

		if el.Epoch.IsZero() {
			t.Error("ParseTLE succeeded with a zero epoch")
		}
	})
}

// FuzzVerifyTLEChecksums is the same property for the other entry point, which
// indexes column 68 of both lines and must not do so before checking there is
// one.
func FuzzVerifyTLEChecksums(f *testing.F) {
	f.Add(issLine1, issLine2)
	f.Add("", "")
	f.Add(issLine1[:68], issLine2[:68])

	f.Fuzz(func(_ *testing.T, line1, line2 string) {
		_ = sgp4.VerifyTLEChecksums(line1, line2)
		_ = sgp4.Checksum(line1)
	})
}
