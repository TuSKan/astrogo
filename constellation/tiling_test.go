package constellation_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
)

// wholeSkyDeg2 is the area of the celestial sphere in square degrees.
const wholeSkyDeg2 = 41252.96124941928

// sweep visits directions spread evenly over the sphere — uniform in sin(dec)
// rather than in dec, so the samples are not piled up at the poles — and calls
// visit for each.
func sweep(nDec, nRA int, visit func(raDeg, decDeg float64)) {
	for i := range nDec {
		sinDec := -1 + 2*(float64(i)+0.5)/float64(nDec)
		dec := math.Asin(sinDec) * 180 / math.Pi

		for j := range nRA {
			visit((float64(j)+0.5)*360/float64(nRA), dec)
		}
	}
}

// Delporte's boundaries tile the sphere, so every direction belongs to exactly
// one constellation and Lookup must never fail.
//
// This is the check that found the boundaries were not being tiled at all.
// Ursa Minor's boundary winds once completely around the north celestial pole,
// which no amount of shifting vertices by whole turns can lay out as a
// contiguous run in right ascension; the planar point-in-polygon test that
// relied on doing so produced a polygon overlapping itself, and with it a hole
// in the sky. A quarter of a per cent of the sphere matched nothing — four per
// cent of the band from +70 to +80 degrees and twenty-three per cent of
// everything above +80 — while other directions up there matched two
// constellations at once.
//
// Spot checks could not have caught it, and did not: the package already
// tested bright stars, the Serpens split, the poles and the right-ascension
// seam, and all of them passed throughout. Only asking whether the whole sky
// is covered finds a hole in it.
func TestBoundariesTileTheWholeSky(t *testing.T) {
	t.Parallel()

	var unresolved int

	var firstFailure string

	sweep(360, 720, func(ra, dec float64) {
		name, abbr, err := constellation.Lookup(coord.NewICRS(angle.Deg(ra), angle.Deg(dec)))
		if err != nil || name == "" || abbr == "" {
			unresolved++

			if firstFailure == "" {
				firstFailure = coord.NewICRS(angle.Deg(ra), angle.Deg(dec)).String()
			}
		}
	})

	if unresolved != 0 {
		t.Errorf("%d directions matched no constellation, the first at %s; Delporte's boundaries "+
			"tile the sphere and Lookup must answer for every direction", unresolved, firstFailure)
	}
}

// Every one of the 88 must actually own some sky. A constellation that is
// never returned is unreachable, whatever the boundary table says.
func TestEveryConstellationIsReachable(t *testing.T) {
	t.Parallel()

	seen := make(map[string]int, 88)

	sweep(360, 720, func(ra, dec float64) {
		if _, abbr, err := constellation.Lookup(coord.NewICRS(angle.Deg(ra), angle.Deg(dec))); err == nil {
			seen[abbr]++
		}
	})

	var missing []string

	for _, c := range constellation.List() {
		if seen[c.Abbreviation] == 0 {
			missing = append(missing, c.Abbreviation)
		}
	}

	if len(missing) != 0 {
		t.Errorf("no direction resolved to %v", missing)
	}

	if len(seen) != 88 {
		t.Errorf("the sweep found %d distinct constellations, want 88", len(seen))
	}
}

// The areas the boundaries enclose must match the published IAU areas.
//
// This is the check that ties the containment test to something outside this
// package. Tiling proves the boundaries cover the sky exactly once; it does not
// prove they are in the right places, and two constellations trading territory
// would satisfy it. The IAU areas are measured, published, and derived from the
// same Delporte boundaries, so reproducing them from a sampled sweep exercises
// the whole path — precession to B1875 included — against numbers this code
// has no other access to.
func TestConstellationAreasMatchThePublishedValues(t *testing.T) {
	t.Parallel()

	const (
		nDec = 720
		nRA  = 1440
	)

	seen := make(map[string]int, 88)

	sweep(nDec, nRA, func(ra, dec float64) {
		if _, abbr, err := constellation.Lookup(coord.NewICRS(angle.Deg(ra), angle.Deg(dec))); err == nil {
			seen[abbr]++
		}
	})

	perSample := wholeSkyDeg2 / float64(nDec*nRA)

	// The three largest, the three smallest, and a few in between. Published
	// IAU areas in square degrees.
	for _, c := range []struct {
		abbr string
		area float64
	}{
		{"Hya", 1302.84},
		{"Vir", 1294.43},
		{"UMa", 1279.66},
		{"Cet", 1231.41},
		{"Ori", 594.12},
		{"UMi", 255.86},
		{"Cir", 93.35},
		{"Sge", 79.93},
		{"Equ", 71.64},
		{"Cru", 68.45},
	} {
		got := float64(seen[c.abbr]) * perSample

		// A sampled area converges as the square root of the sample count; at
		// this resolution two per cent is comfortable for the small ones and
		// far tighter than any error that would matter.
		if rel := math.Abs(got-c.area) / c.area; rel > 0.02 {
			t.Errorf("%s encloses %.2f square degrees, want the published %.2f (%.2f%% out)",
				c.abbr, got, c.area, 100*rel)
		}
	}

	// And the whole sky must be accounted for, once.
	var total int
	for _, n := range seen {
		total += n
	}

	if total != nDec*nRA {
		t.Errorf("the constellations account for %d of %d sampled directions", total, nDec*nRA)
	}
}

// The north polar cap is the region the boundary catalogue leaves implicit,
// and the one that was broken. Ursa Minor's boundary tops out at +88 degrees
// and encircles the pole above that.
func TestNorthPolarCapBelongsToUrsaMinor(t *testing.T) {
	t.Parallel()

	// From +89 upward, since these are J2000 positions and the boundaries are
	// B1875: precession moves a direction by well over a degree between the
	// two epochs, so J2000 +88.5 at right ascension zero lands at B1875 +87.80,
	// which is genuinely inside Cepheus. Cepheus really does reach +88 there.
	for _, dec := range []float64{89, 89.5, 89.9, 89.99, 90} {
		for _, ra := range []float64{0, 45, 90, 135, 180, 225, 270, 315, 359.9} {
			name, abbr, err := constellation.Lookup(coord.NewICRS(angle.Deg(ra), angle.Deg(dec)))
			if err != nil {
				t.Errorf("RA %.1f Dec %+.2f: %v", ra, dec, err)

				continue
			}

			if abbr != "UMi" {
				t.Errorf("RA %.1f Dec %+.2f resolved to %s (%s), want Ursa Minor", ra, dec, name, abbr)
			}
		}
	}

	// Polaris is the anchor: a real star, unambiguously in Ursa Minor, close
	// enough to the pole to sit inside the cap that was broken.
	if _, abbr, err := constellation.Lookup(
		coord.NewICRS(angle.Deg(37.9545), angle.Deg(89.2641))); err != nil || abbr != "UMi" {
		t.Errorf("Polaris resolved to %q (err %v), want UMi", abbr, err)
	}

	// The south polar cap is Octans, which the catalogue does close explicitly
	// and which therefore never had the same problem — checked so that a
	// change to the polar handling cannot break it unnoticed.
	for _, dec := range []float64{-88.5, -89.5, -89.99, -90} {
		for _, ra := range []float64{0, 90, 180, 270} {
			name, abbr, err := constellation.Lookup(coord.NewICRS(angle.Deg(ra), angle.Deg(dec)))
			if err != nil {
				t.Errorf("RA %.1f Dec %+.2f: %v", ra, dec, err)

				continue
			}

			if abbr != "Oct" {
				t.Errorf("RA %.1f Dec %+.2f resolved to %s (%s), want Octans", ra, dec, name, abbr)
			}
		}
	}
}
