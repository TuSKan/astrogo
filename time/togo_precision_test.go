package time_test

import (
	"math/rand/v2"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/time"
)

// TestToGoRoundTripsTheNanosecond: a standard-library time is an integer
// count of nanoseconds, and FromGo keeps it to a few picoseconds, so ToGo must
// give it back exactly. Until #420 ToGo summed the two Julian-date parts into
// one float64 count of seconds since 1970, whose 238 ns spacing in 2026 lost
// up to 119 ns: 2026-09-23 12:34:56.123456789 came back 72 ns early, and 199,117
// of 200,000 random 2026 instants came back different. Whole minutes came back
// exact, which is how it went unseen.
func TestToGoRoundTripsTheNanosecond(t *testing.T) {
	in := gotime.Date(2026, 9, 23, 12, 34, 56, 123456789, gotime.UTC)
	if out := time.FromGo(in).ToGo(); !out.Equal(in) {
		t.Errorf("FromGo(%v).ToGo() = %v, off by %v", in, out, out.Sub(in))
	}

	rng := rand.New(rand.NewPCG(420, 2026))
	lo := gotime.Date(1900, 1, 1, 0, 0, 0, 0, gotime.UTC).Unix()
	hi := gotime.Date(2100, 1, 1, 0, 0, 0, 0, gotime.UTC).Unix()

	wrong := 0

	for range 100_000 {
		in := gotime.Unix(lo+rng.Int64N(hi-lo), rng.Int64N(1e9)).UTC()

		if out := time.FromGo(in).ToGo(); !out.Equal(in) {
			if wrong++; wrong <= 5 {
				t.Errorf("FromGo(%v).ToGo() = %v, off by %v", in, out, out.Sub(in))
			}
		}
	}

	if wrong > 0 {
		t.Errorf("%d of 100,000 instants over 1900-2100 did not round-trip", wrong)
	}
}

// TestToGoRoundTripsAtTheEdges covers what random instants rarely reach: the
// last nanosecond of a day, of a day ending in a leap second, before 1970,
// and in the first century, where the seconds since 1970 are 6e10.
func TestToGoRoundTripsAtTheEdges(t *testing.T) {
	for _, in := range []gotime.Time{
		gotime.Date(2026, 9, 23, 23, 59, 59, 999999999, gotime.UTC),
		gotime.Date(2026, 9, 24, 0, 0, 0, 1, gotime.UTC),
		gotime.Date(2016, 12, 31, 23, 59, 59, 999999999, gotime.UTC),
		gotime.Date(2016, 12, 31, 12, 0, 0, 123456789, gotime.UTC),
		gotime.Date(1969, 12, 31, 23, 59, 59, 999999999, gotime.UTC),
		gotime.Date(1970, 1, 1, 0, 0, 0, 0, gotime.UTC),
		gotime.Date(33, 4, 3, 15, 0, 0, 987654321, gotime.UTC),
		gotime.Date(2999, 6, 30, 6, 30, 30, 500000001, gotime.UTC),
	} {
		if out := time.FromGo(in).ToGo(); !out.Equal(in) {
			t.Errorf("FromGo(%v).ToGo() = %v, off by %v", in, out, out.Sub(in))
		}
	}
}
