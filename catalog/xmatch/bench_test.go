package xmatch

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

// allSky is two catalogs of n positions each, uniform over the sky, sharing
// no identifier, with every tenth row of b a re-observation of a row of a
// 0.3″ away.
func allSky(n int) (a, b []resolve.Target) {
	rng := rand.New(rand.NewPCG(1183, 2))
	sky := func() coord.ICRS {
		return coord.NewICRS(angle.Rad(rng.Float64()*2*math.Pi), angle.Rad(math.Asin(rng.Float64()*2-1)))
	}

	a, b = make([]resolve.Target, n), make([]resolve.Target, n)
	for k := range n {
		a[k] = resolve.Target{HasCoord: true, Coord: sky()}
		b[k] = resolve.Target{HasCoord: true, Coord: sky()}
	}

	for k := 0; k < n; k += 10 {
		b[k].Coord = coord.NewICRS(a[k].Coord.RA(), a[k].Coord.Dec().Add(angle.Arcsec(0.3)))
	}

	return a, b
}

// BenchmarkMatchAllSky matches two all-sky catalogs of 20,000 rows at a 1″
// resolution: the positional pass compares only the rows its index finds
// near each other, not the 400 million pairs.
func BenchmarkMatchAllSky(b *testing.B) {
	ca, cb := allSky(20000)
	opt := WithResolution(angle.Arcsec(1))

	for b.Loop() {
		if pairs := Match(ca, cb, opt); len(pairs) < 2000 {
			b.Fatalf("%d pairs, want the 2000 planted", len(pairs))
		}
	}
}
