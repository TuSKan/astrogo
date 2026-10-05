package time_test

import (
	"math"
	"testing"

	"github.com/hebl/gofa"

	"github.com/TuSKan/astrogo/time"
)

// TestLabelAnUlpBelowLeapMidnight is #499: a UTC label within an ulp below the
// midnight that ends a leap second was read as the next day's start, without
// the second, and converted to TAI a whole second early. It must convert as
// iauUtctai does, and TAI must come back to it.
func TestLabelAnUlpBelowLeapMidnight(t *testing.T) {
	t.Parallel()

	// 2017-01-01 0h UTC, the end of the 2016-12-31 leap second, less 26 ns.
	jd1, jd2 := 2457754.5, -3e-13

	var a1, a2 float64
	if gofa.Utctai(jd1, jd2, &a1, &a2) < 0 {
		t.Fatal("Utctai")
	}

	utc := time.FromJDParts(jd1, jd2, time.UTC)

	g1, g2 := utc.TAI().JDParts()
	if d := ((g1 - a1) + (g2 - a2)) * 86400; math.Abs(d) > 1e-9 {
		t.Fatalf("TAI is %.9f s from iauUtctai's", d)
	}

	b1, b2 := time.FromJDParts(a1, a2, time.TAI).UTC().JDParts()
	if d := ((b1 - jd1) + (b2 - jd2)) * 86400; math.Abs(d) > 1e-9 {
		t.Errorf("TAI back to UTC lands %.9f s from where it started", d)
	}
}
