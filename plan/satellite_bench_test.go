package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
)

// benchISS builds the provider and site the satellite benchmarks share.
//
// Same real ISS TLE the offline tests use, so nothing here touches the network.
func benchISS(b *testing.B) (*satellite.Satellite, *coord.Geodetic) {
	b.Helper()

	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		b.Fatalf("NewFromTLE: %v", err)
	}

	site, err := coord.NewGeodetic(angle.Deg(-70.4028), angle.Deg(-24.6251), 2635)
	if err != nil {
		b.Fatalf("NewGeodetic: %v", err)
	}

	return sat, site
}

// BenchmarkLookAngle measures one look-angle evaluation against a Context the
// caller already holds.
//
// This is the call that used to discard that Context and have coord.Reducer
// build a second one, repeating the Apco13 solve. The Context is constructed
// outside the loop precisely because that is how a caller uses it — the point
// of taking one is that the expensive part is already paid for.
func BenchmarkLookAngle(b *testing.B) {
	sat, site := benchISS(b)

	when := time.Date(2026, time.April, 20, 3, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(when, site, defaultAtm)

	b.ResetTimer()

	for b.Loop() {
		if _, err := LookAngle(sat, 0, ctx); err != nil {
			b.Fatalf("LookAngle: %v", err)
		}
	}
}

// BenchmarkSatellitePasses_6h is the workload the cost actually lands in:
// 30-second sampling across a six-hour window, which is 720 look angles plus
// the root-finding refinement on each crossing.
//
// At one Apco13 solve per sample this is the floor; it used to be two.
func BenchmarkSatellitePasses_6h(b *testing.B) {
	sat, site := benchISS(b)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(6 * time.Hour)

	b.ResetTimer()

	for b.Loop() {
		if _, err := SatellitePasses(sat, "ISS", start, end, site, angle.Deg(10)); err != nil {
			b.Fatalf("SatellitePasses: %v", err)
		}
	}
}
