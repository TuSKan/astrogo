package constellation

import (
	"errors"

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

	return result
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
		if containsPoint(l.points, raHours, decDeg) {
			n, ok := names[l.key]
			if !ok {
				continue
			}

			return n.full, n.abbr, nil
		}
	}

	// The catalog's Ursa Minor boundary tops out at Dec +88°, not +90° —
	// the small remaining north-polar cap has no explicit boundary in the
	// source data, and by long-established convention (the same fallback
	// the standard reference algorithm for this catalog uses) belongs to
	// Ursa Minor. This is the only region where Lookup can fail to match
	// any polygon; anywhere else, "no match" is a genuine bug.
	if decDeg > 88 {
		return "Ursa Minor", "UMi", nil
	}

	return "", "", ErrNoMatch
}

// containsPoint reports whether (ra, dec) — ra in hours, dec in degrees —
// falls inside the closed polygon poly, via the standard even-odd
// ray-casting rule (a horizontal ray cast in the +RA direction). RA wraps
// at 24h: poly's own points are unwrapped into a contiguous run first (any
// gap between consecutive vertices wider than 12h is assumed to be a
// wraparound, not a genuine jump), and the query RA is tried at its
// original value and both ±24h shifts so it lines up with whichever
// contiguous run the polygon was unwrapped into.
func containsPoint(poly []point, ra, dec float64) bool {
	unwrapped := unwrapRA(poly)

	for _, shift := range [3]float64{0, 24, -24} {
		if rayCast(unwrapped, ra+shift, dec) {
			return true
		}
	}

	return false
}

// unwrapRA returns poly's points with RA adjusted by ±24h wherever needed
// so consecutive vertices never jump by more than 12h — turning a boundary
// that crosses the 0h/24h seam into one contiguous run suitable for planar
// point-in-polygon testing.
func unwrapRA(poly []point) []point {
	out := make([]point, len(poly))
	out[0] = poly[0]

	for i := 1; i < len(poly); i++ {
		ra := poly[i].ra
		prev := out[i-1].ra

		for ra-prev > 12 {
			ra -= 24
		}

		for ra-prev < -12 {
			ra += 24
		}

		out[i] = point{ra, poly[i].dec}
	}

	return out
}

// rayCast is the classic even-odd point-in-polygon test: casts a ray from
// (ra, dec) in the increasing-RA direction and counts edge crossings.
func rayCast(poly []point, ra, dec float64) bool {
	inside := false

	n := len(poly)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		pi, pj := poly[i], poly[j]

		if (pi.dec > dec) != (pj.dec > dec) {
			raAtDec := pi.ra + (dec-pi.dec)/(pj.dec-pi.dec)*(pj.ra-pi.ra)
			if ra < raAtDec {
				inside = !inside
			}
		}
	}

	return inside
}
