package starlight_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// patternedMap builds an order-4 map whose value encodes its own pixel index,
// so a lookup that lands on the wrong pixel is visible as a wrong number rather
// than as a plausible one.
func patternedMap(t *testing.T) *starlight.Map {
	t.Helper()

	const npix = 12 * 4 * 4 * 4 * 4 // order 4

	values := make([]float64, npix)
	for i := range values {
		values[i] = 1e-12 * float64(i+1)
	}

	m, err := starlight.NewMap(starlight.ICRS, map[string][]float64{"V": values})
	if err != nil {
		t.Fatalf("NewMap: %v", err)
	}

	return m
}

// The sky is a sphere, so the lookup has to close at the seam.
//
// Right ascension 0 and 360 are the same direction, and so are −1 and 359. A
// lookup that does not wrap lands on a different pixel or falls outside the map
// entirely, and on a real map the value it returns is an ordinary radiance from
// somewhere else on the sky — plausible, and wrong.
func TestRadianceAtClosesAtTheRAWrap(t *testing.T) {
	t.Parallel()

	m := patternedMap(t)

	at := func(raDeg, decDeg float64) float64 {
		v, err := m.RadianceAt("V", angle.Deg(raDeg), angle.Deg(decDeg))
		if err != nil {
			t.Fatalf("RadianceAt(%.3f, %.3f): %v", raDeg, decDeg, err)
		}

		return v
	}

	for _, dec := range []float64{-75, -30, 0, 30, 75} {
		if a, b := at(0, dec), at(360, dec); a != b {
			t.Errorf("dec %+.0f: RA 0 gives %.6e and RA 360 gives %.6e", dec, a, b)
		}

		if a, b := at(-1, dec), at(359, dec); a != b {
			t.Errorf("dec %+.0f: RA -1 gives %.6e and RA 359 gives %.6e", dec, a, b)
		}

		if a, b := at(720+45, dec), at(45, dec); a != b {
			t.Errorf("dec %+.0f: RA 765 gives %.6e and RA 45 gives %.6e", dec, a, b)
		}
	}
}

// A pole lookup must succeed and be stable, and it is not single-valued.
//
// The first version of this test asserted that every azimuth at declination 90
// returns the same value, on the reasoning that the pole is one direction. That
// is true of the sky and false of the pixelation: HEALPix has no pixel centred
// on a pole, four meet there, and which one a lookup lands in depends on the
// longitude it was given. The values differ by however much the sky differs
// across 13.7 arcminutes at order 8, which is resolution rather than error.
//
// What can be asserted is that the lookup resolves, that it is repeatable, and
// that the whole ring of azimuths lands on a small number of distinct pixels
// rather than being scattered across the map — which is what a longitude
// mishandled at the pole would do.
func TestRadianceAtThePolesIsStableAndLocal(t *testing.T) {
	t.Parallel()

	m := patternedMap(t)

	for _, dec := range []float64{90, -90} {
		seen := map[float64]int{}

		for _, ra := range []float64{0, 30, 45, 90, 137, 180, 225, 271, 315, 359.99} {
			first, err := m.RadianceAt("V", angle.Deg(ra), angle.Deg(dec))
			if err != nil {
				t.Fatalf("RadianceAt(%.2f, %+.0f): %v", ra, dec, err)
			}

			again, err := m.RadianceAt("V", angle.Deg(ra), angle.Deg(dec))
			if err != nil {
				t.Fatalf("RadianceAt repeat: %v", err)
			}

			if first != again {
				t.Errorf("dec %+.0f RA %.2f is not repeatable: %.6e then %.6e", dec, ra, first, again)
			}

			seen[first]++
		}

		// Four pixels meet at a pole. Ten azimuths hitting more than four
		// distinct pixels means the longitude is doing something other than
		// choosing between them.
		if len(seen) > 4 {
			t.Errorf("dec %+.0f: ten azimuths landed on %d distinct pixels, want at most the four "+
				"that meet at a pole", dec, len(seen))
		}
	}
}

// Every direction on the sphere must resolve to a pixel inside the map.
//
// ErrPixelRange exists for an index outside the array, and no real viewing
// direction should ever produce one. This sweeps the sphere finely enough to
// cross every base-pixel boundary, including the ones at the poles and along
// the equatorial seams where the HEALPix index changes scheme.
func TestEveryDirectionResolvesToAPixel(t *testing.T) {
	t.Parallel()

	m := patternedMap(t)

	for decStep := range 181 {
		dec := float64(decStep) - 90

		for raStep := range 121 {
			ra := float64(raStep) * 3

			v, err := m.RadianceAt("V", angle.Deg(ra), angle.Deg(dec))
			if err != nil {
				t.Fatalf("RA %.1f dec %+.1f fell outside the map: %v", ra, dec, err)
			}

			if v <= 0 || math.IsNaN(v) {
				t.Fatalf("RA %.1f dec %+.1f gave %v", ra, dec, v)
			}
		}
	}
}

// A declination outside the sphere is a caller error, not a wrap.
//
// Latitude does not wrap the way longitude does: 91 degrees north is not 89
// degrees on the far side for a map lookup, it is a mistake. Whatever the grid
// does with it, the result must not be silently taken as a real direction.
func TestRadianceAtRejectsOrClampsImpossibleDeclinations(t *testing.T) {
	t.Parallel()

	m := patternedMap(t)

	for _, dec := range []float64{90.001, 120, 180, -90.001, -120} {
		v, err := m.RadianceAt("V", angle.Deg(10), angle.Deg(dec))
		if err != nil {
			continue // rejected, which is a fine answer
		}

		// Accepted, so it must at least have clamped to the pole rather than
		// wrapped to some unrelated part of the sky.
		pole, poleErr := m.RadianceAt("V", angle.Deg(10), angle.Deg(math.Copysign(90, dec)))
		if poleErr != nil {
			t.Fatalf("pole lookup: %v", poleErr)
		}

		if v != pole {
			t.Errorf("dec %+.3f was accepted and gave %.6e, which is neither an error "+
				"nor the pole's %.6e", dec, v, pole)
		}
	}
}

// An unknown band is an error, not a zero.
//
// A map holds the bands it was built with. Asking for another one and getting
// zero back would read downstream as a dark sky rather than as a mistake.
func TestRadianceAtRejectsAnUnknownBand(t *testing.T) {
	t.Parallel()

	m := patternedMap(t)

	if _, err := m.RadianceAt("R", angle.Deg(10), angle.Deg(10)); err == nil {
		t.Error("a band the map does not hold returned a value")
	}
}
