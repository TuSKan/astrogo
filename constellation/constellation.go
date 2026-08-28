package constellation

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/vector"
)

// ErrNoMatch indicates a position matched no constellation boundary — this
// should not happen for any finite, valid ICRS position, since Delporte's
// boundaries tile the whole sky with no gaps.
var ErrNoMatch = errors.New("constellation: no boundary matched")

// point is one B1875-epoch (RA hours, Dec degrees) boundary vertex.
type point struct {
	ra, dec float64
}

// loop is one constellation's closed boundary (the last point implicitly
// connects back to the first).
type loop struct {
	key    string // raw catalog abbreviation, e.g. "AND", "SER1"
	points []point

	// polar records that this boundary winds once around the north
	// celestial pole, which inverts the parity of the containment test.
	// Computed once at startup rather than on every lookup.
	polar bool
}

// loops groups rawBoundary's contiguous same-constellation runs into closed
// boundary loops, computed once from the embedded catalog data.
var loops = groupIntoLoops(rawBoundary)

func groupIntoLoops(raw []struct {
	raHours float64
	decDeg  float64
	abbr    string
},
) []loop {
	var result []loop

	for _, p := range raw {
		if n := len(result); n > 0 && result[n-1].key == p.abbr {
			result[n-1].points = append(result[n-1].points, point{p.raHours, p.decDeg})
			continue
		}

		result = append(result, loop{key: p.abbr, points: []point{{p.raHours, p.decDeg}}})
	}

	for i := range result {
		result[i].polar = windsAroundNorthPole(result[i].points)
	}

	return result
}

// windsAroundNorthPole reports whether poly's boundary encircles the north
// celestial pole, by summing the signed right ascension travelled around the
// closed loop. A boundary enclosing no pole returns to where it started and
// sums to zero; one that encircles a pole sums to a full turn.
//
// Ursa Minor is the only northern constellation this holds for, and Octans the
// only southern one. Octans is excluded by the declination test, because a ray
// cast northward from inside it leaves through its own northern boundary in
// the ordinary way and needs no special case.
func windsAroundNorthPole(poly []point) bool {
	var (
		winding float64
		maxDec  = -91.0
	)

	n := len(poly)
	for i := range n {
		a, b := poly[i], poly[(i+1)%n]

		if a.dec > maxDec {
			maxDec = a.dec
		}

		// The step between consecutive vertices is always the shorter way
		// round: no boundary segment spans half the sky.
		d := b.ra - a.ra
		for d > 12 {
			d -= 24
		}

		for d < -12 {
			d += 24
		}

		winding += d
	}

	return math.Abs(winding) > 12 && maxDec > 0
}

// b1875Matrix is the IAU 1976 precession rotation matrix from J2000.0 to
// B1875.0 — the equinox Delporte's official boundaries are defined
// against. Computed once from the two fixed epochs involved, not per call.
var b1875Matrix = func() [3][3]float64 {
	djm0, djm := gofaext.Epb2jd(1875.0)
	return gofaext.Pmat76(djm0, djm)
}()

// Lookup returns the IAU constellation name and standard 3-letter
// abbreviation containing pos (a J2000 ICRS position).
//
// pos is precessed to the B1875.0 equinox first (via the IAU 1976
// precession model), since that's the epoch Delporte's boundaries are
// defined against — comparing a J2000 position directly against B1875
// boundaries would be wrong by the roughly 1.4° of precessional drift
// accumulated since 1875. Containment is then tested directly against each
// constellation's boundary polygon in (RA, Dec) coordinate space — correct
// here specifically because Delporte's boundaries are defined as RA/Dec
// grid lines (constant right ascension or constant declination), never
// diagonal or great-circle arcs, so an ordinary planar point-in-polygon
// test against the raw (RA, Dec) vertices is exact, not an approximation.
func Lookup(pos coord.ICRS) (name, abbreviation string, err error) {
	v := pos.ToUnitVector()
	v1875 := gofaext.Rxp(b1875Matrix, [3]float64{v.X, v.Y, v.Z})

	var precessed coord.ICRS

	precessed.FromUnitVector(vector.V3(v1875[0], v1875[1], v1875[2]))

	raHours := precessed.RA().Degrees() / 15
	decDeg := precessed.Dec().Degrees()

	for _, l := range loops {
		if containsPoint(l.points, l.polar, raHours, decDeg) {
			n, ok := names[l.key]
			if !ok {
				continue
			}

			return n.full, n.abbr, nil
		}
	}

	return "", "", ErrNoMatch
}

// containsPoint reports whether (ra, dec) - ra in hours, dec in degrees -
// lies inside the closed boundary poly.
//
// It counts the boundary segments a meridian ray cast northward from the point
// crosses. Delporte's boundaries run only along lines of constant right
// ascension or constant declination, so a northward ray can cross only a
// constant-declination segment, and whether it does is settled exactly by
// comparing that segment's own right-ascension span against the point's. There
// is no interpolation and nothing that depends on laying the polygon out as
// one contiguous run first.
//
// That last point is why this replaced a planar test in unwrapped right
// ascension. Ursa Minor's boundary winds once completely around the north
// celestial pole: east from 13h to 23h at rising declination, across the 0h
// seam at +88 degrees, and back to 13h. No shift of any vertex by whole turns
// lays that out contiguously - the honest span is 24 hours and the unwrapping
// produced 25.5, a polygon overlapping itself. The result was a hole in the
// sky. A quarter of a per cent of the sphere matched no constellation at all:
// four per cent of the band from +70 to +80 degrees, and twenty-three per cent
// of everything above +80, while other directions up there matched two
// constellations at once.
//
// A ray cast northward from inside a pole-encircling boundary leaves through
// the pole rather than through the boundary, so the parity is inverted for
// those. polar records which they are.
func containsPoint(poly []point, polar bool, ra, dec float64) bool {
	crossings := 0

	n := len(poly)
	for i := range n {
		a, b := poly[i], poly[(i+1)%n]

		// A constant-right-ascension segment runs parallel to the ray, and a
		// segment at or below the point is not in front of it.
		if a.dec != b.dec || a.dec <= dec {
			continue
		}

		if meridianCrosses(a.ra, b.ra, ra) {
			crossings++
		}
	}

	if polar {
		return crossings%2 == 0
	}

	return crossings%2 == 1
}

// meridianCrosses reports whether the meridian at ra passes through the
// constant-declination segment joining raA and raB.
//
// The span is half-open, so two segments meeting at a shared vertex are
// counted once rather than twice or not at all.
func meridianCrosses(raA, raB, ra float64) bool {
	lo, hi := raA, raB
	if lo > hi {
		lo, hi = hi, lo
	}

	// No boundary segment spans half the sky, so a direct span wider than
	// twelve hours means the segment is the other arc - the one crossing the
	// 0h/24h seam, covering [hi, 24) together with [0, lo).
	if hi-lo > 12 {
		return ra >= hi || ra < lo
	}

	return ra >= lo && ra < hi
}
