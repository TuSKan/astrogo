package constellation

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/vector"
)

// ErrUnknownAbbreviation indicates Centroid was given a name/abbreviation
// this package has no boundary data for.
var ErrUnknownAbbreviation = errors.New("constellation: unknown name or abbreviation")

// Constellation is a lightweight name/abbreviation descriptor — see List.
type Constellation struct {
	// Name is the full IAU constellation name (e.g. "Orion").
	Name string
	// Abbreviation is the standard 3-letter IAU abbreviation (e.g. "Ori").
	Abbreviation string
}

// List returns every constellation this package has boundary data for,
// sorted by Name — the 88 official IAU constellations. The underlying
// catalog carries 89 raw boundary-loop keys, not 88, since Serpens is
// split by Ophiuchus into two disjoint regions (Serpens Caput/Cauda,
// internal catalog keys "SER1"/"SER2") that are one constellation, not
// two; List unifies them into a single "Serpens" entry, and Centroid
// treats it the same way, averaging vertices from both parts.
func List() []Constellation {
	seen := make(map[string]bool, len(names))
	out := make([]Constellation, 0, len(names))

	for _, n := range names {
		if seen[n.abbr] {
			continue
		}

		seen[n.abbr] = true

		out = append(out, Constellation{Name: n.full, Abbreviation: n.abbr})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// normalize lowercases and strips spaces for case/space-insensitive
// name/abbreviation matching — same convention as plan's own
// normalizeSiteName.
func normalize(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", ""))
}

// Centroid returns a rough "where to look" point for the constellation
// matched by name (its full IAU name or 3-letter abbreviation,
// case/space-insensitive) — the mean of its boundary polygon's vertices,
// precessed from the catalog's native B1875.0 equinox back to J2000 ICRS.
//
// This is NOT a rigorously defined "center": no single agreed definition
// exists for an irregular polygon's centroid on a sphere (a vertex average
// is not area-weighted), and for Serpens specifically it mixes vertices
// from two disjoint regions (Serpens Caput and Cauda) into one point that
// may not usefully represent either. Good enough for "point roughly this
// way," not for anything requiring precision.
func Centroid(name string) (coord.ICRS, error) {
	want := normalize(name)

	var (
		sum [3]float64
		n   int
	)

	for _, l := range loops {
		info, ok := names[l.key]
		if !ok {
			continue
		}

		if normalize(l.key) != want && normalize(info.abbr) != want && normalize(info.full) != want {
			continue
		}

		for _, p := range l.points {
			v := vector.FromSpherical(angle.Hour(p.ra).Radians(), angle.Deg(p.dec).Radians())
			sum[0] += v.X
			sum[1] += v.Y
			sum[2] += v.Z
		}

		n += len(l.points)
	}

	if n == 0 {
		return coord.ICRS{}, fmt.Errorf("%w: %q", ErrUnknownAbbreviation, name)
	}

	meanB1875 := vector.V3(sum[0]/float64(n), sum[1]/float64(n), sum[2]/float64(n)).Unit()
	j2000 := trxp(b1875Matrix, [3]float64{meanB1875.X, meanB1875.Y, meanB1875.Z})

	var out coord.ICRS

	out.FromUnitVector(vector.V3(j2000[0], j2000[1], j2000[2]))

	return out, nil
}

// trxp computes Rᵀ·p — the inverse of gofaext.Rxp's R·p for an orthogonal
// rotation matrix (transpose = inverse) — used here to precess a B1875
// boundary vertex back to J2000/ICRS, the reverse of Lookup's own
// J2000→B1875 rotation via b1875Matrix.
func trxp(r [3][3]float64, p [3]float64) [3]float64 {
	return [3]float64{
		r[0][0]*p[0] + r[1][0]*p[1] + r[2][0]*p[2],
		r[0][1]*p[0] + r[1][1]*p[1] + r[2][1]*p[2],
		r[0][2]*p[0] + r[1][2]*p[1] + r[2][2]*p[2],
	}
}
