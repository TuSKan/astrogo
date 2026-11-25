package ephemeris_test

import (
	"testing"
	"time"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
)

// BenchmarkDefaultPerCallAtOneInstant is what plan's Sun, MoonSep and
// MoonIllum constraints did on every check: build eph.Default() and ask it for
// the Sun from the Earth, at an instant the previous check had just asked
// about. With a SOFA base per provider every call paid a full Epv00, about
// 30 µs. With the base shared it is the memo's answer (#913).
func BenchmarkDefaultPerCallAtOneInstant(b *testing.B) {
	t := epoch.Date(2026, time.March, 20, 22, 0, 0, 0, time.UTC)

	for b.Loop() {
		if _, err := eph.At(eph.Default(), t).Geometric(eph.Sun, eph.Earth); err != nil {
			b.Fatal(err)
		}
	}
}
