package dust

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// testHemisphere builds a polar projection with the real map's geometry and a
// value in every pixel encoding where it is, so a lookup can be checked
// against the position it came from.
func testHemisphere(nsgp float64) *hemisphere {
	const side = 4096

	h := &hemisphere{
		values: make([]float64, side*side),
		width:  side,
		height: side,
		nsgp:   nsgp,
		scale:  2048,
		crpix1: 2048.50,
		crpix2: 2048.50,
	}

	for y := range side {
		for x := range side {
			h.values[y*side+x] = float64(x) + 10000*float64(y)
		}
	}

	return h
}

// The pole lands at the centre and the galactic equator at the map's edge.
//
// Those are the two ends of the radial coordinate, and between them they pin
// the scale. Getting sqrt(1 - sin b) wrong — using the zenith angle, say —
// leaves both endpoints right and everything between them wrong, which is why
// the middle is checked too.
func TestSFDProjectionRadialScale(t *testing.T) {
	t.Parallel()

	h := testHemisphere(+1)

	for _, c := range []struct {
		name  string
		bDeg  float64
		wantR float64
	}{
		{"the north galactic pole is the centre", 90, 0},
		{"the galactic equator is at the full scale", 0, 2048},
		{"30 degrees is sqrt(1/2) of the way out", 30, 2048 * math.Sqrt2 / 2},
		{"60 degrees", 60, 2048 * math.Sqrt(1-math.Sqrt(3)/2)},
	} {
		// Measured from the pole, which sits at CRPIX minus one in
		// zero-indexed pixels.
		x, y := h.pixelOf(angle.Deg(0), angle.Deg(c.bDeg))
		got := math.Hypot(x-(h.crpix1-1), y-(h.crpix2-1))

		if math.Abs(got-c.wantR) > 1e-9 {
			t.Errorf("%s: radius %.6f pixels, want %.6f", c.name, got, c.wantR)
		}
	}
}

// The projection is equal-area, which is what makes averaging over pixels the
// same as averaging over sky.
//
// Checked directly rather than taken on trust: the fraction of the map's area
// inside a given galactic latitude must equal the fraction of the hemisphere's
// solid angle inside it. Above latitude b the cap covers 1 - sin(b) of the
// hemisphere, and a disc of radius r covers r^2/scale^2 of the map.
//
// This matters beyond tidiness. The one improvement docs/skybrightness.md
// records for the scattering integral is averaging the incoming field over a
// quadrature cell instead of sampling its centre, and that is only a mean over
// sky if the pixels carry equal solid angle.
func TestSFDProjectionIsEqualArea(t *testing.T) {
	t.Parallel()

	h := testHemisphere(+1)

	for _, bDeg := range []float64{5, 15, 30, 45, 60, 75, 89} {
		x, y := h.pixelOf(angle.Deg(137), angle.Deg(bDeg))
		r := math.Hypot(x-(h.crpix1-1), y-(h.crpix2-1))

		areaFraction := (r * r) / (h.scale * h.scale)
		solidAngleFraction := 1 - math.Sin(bDeg*math.Pi/180)

		if math.Abs(areaFraction-solidAngleFraction) > 1e-12 {
			t.Errorf("at b = %g: the disc covers %.12f of the map and the cap %.12f of the "+
				"hemisphere; the projection is not equal-area",
				bDeg, areaFraction, solidAngleFraction)
		}
	}
}

// Longitude runs the right way round, and the two hemispheres run opposite
// ways.
//
// The sign on Y carries NSGP. Getting it wrong mirrors the sky: every lookup
// still lands somewhere real and plausible, and the Milky Way comes out
// reflected — the class of error that survives every check except one aimed
// at it.
func TestSFDProjectionWindingFollowsHemisphere(t *testing.T) {
	t.Parallel()

	north := testHemisphere(+1)
	south := testHemisphere(-1)

	// At l = 90 the displacement from the pole is purely in Y, so its sign is
	// the winding and nothing else.
	_, yNorth := north.pixelOf(angle.Deg(90), angle.Deg(45))
	_, ySouth := south.pixelOf(angle.Deg(90), angle.Deg(-45))

	dNorth := yNorth - (north.crpix2 - 1)
	dSouth := ySouth - (south.crpix2 - 1)

	if dNorth == 0 || dSouth == 0 {
		t.Fatalf("l = 90 should displace purely in Y: %g and %g", dNorth, dSouth)
	}

	if math.Signbit(dNorth) == math.Signbit(dSouth) {
		t.Errorf("both hemispheres wind the same way (%g and %g); the NSGP sign is not "+
			"being carried and one of them is mirrored", dNorth, dSouth)
	}
}

// A lookup interpolates between pixels rather than snapping to one.
//
// The map is consulted at arbitrary directions and its pixels are 2.37
// arcminutes. Nearest-neighbour would make the intensity a step function of
// direction, and a component integrating over the sky would then see edges
// belonging to the sampling rather than to the dust.
func TestSFDLookupInterpolates(t *testing.T) {
	t.Parallel()

	h := testHemisphere(+1)

	at := func(x, y float64) float64 {
		t.Helper()

		v, err := h.bilinear(x, y)
		if err != nil {
			t.Fatalf("bilinear: %v", err)
		}

		return v
	}

	// The synthetic map holds x + 10000y, so a half-pixel step in x must move
	// the answer by half a unit and one in y by five thousand.
	whole := at(100, 200)

	if step := at(100.5, 200) - whole; math.Abs(step-0.5) > 1e-9 {
		t.Errorf("a half-pixel step in x gave %g, want 0.5 — the lookup is not "+
			"interpolating", step)
	}

	if step := at(100, 200.5) - whole; math.Abs(step-5000) > 1e-9 {
		t.Errorf("a half-pixel step in y gave %g, want 5000", step)
	}

	if step := at(100.5, 200.5) - whole; math.Abs(step-5000.5) > 1e-9 {
		t.Errorf("a half-pixel diagonal gave %g, want 5000.5", step)
	}
}

// A direction outside the map is clamped to its edge rather than read out of
// bounds.
//
// Each file covers its own hemisphere and a little past the equator, and
// IntensityAt picks the right one, so this should not be reachable. It is
// still worth holding: a lookup that indexed outside the array would panic,
// and a dust map is not worth a panic.
func TestSFDLookupStaysInBounds(t *testing.T) {
	t.Parallel()

	h := testHemisphere(+1)

	for _, c := range []struct{ x, y float64 }{
		{-100, -100}, {1e9, 1e9}, {-1, 2000}, {2000, -1},
		{float64(h.width), 10}, {10, float64(h.height)},
	} {
		if _, err := h.bilinear(c.x, c.y); err != nil {
			t.Errorf("(%g, %g): %v", c.x, c.y, err)
		}
	}

	if _, err := h.bilinear(math.NaN(), 0); err == nil {
		t.Error("a NaN position was accepted")
	}
}

// IntensityAt reads each hemisphere from its own file.
//
// Both files reach a little past the equator, so a direction near b = 0 is in
// both. Taking the one it belongs to keeps every lookup inside its own
// projection rather than out at the rim, where the sampling is coarsest.
func TestSFDPicksTheHemisphere(t *testing.T) {
	t.Parallel()

	// Distinguishable hemispheres: north holds its pixel encoding, south holds
	// a constant nothing in the north can produce.
	north := testHemisphere(+1)
	south := testHemisphere(-1)

	for i := range south.values {
		south.values[i] = -1
	}

	s := &SFD{north: north, south: south}

	for _, c := range []struct {
		bDeg  float64
		south bool
	}{
		{45, false}, {0.1, false}, {0, false}, {-0.1, true}, {-45, true},
	} {
		got, err := s.IntensityAt(angle.Deg(120), angle.Deg(c.bDeg))
		if err != nil {
			t.Fatalf("b = %g: %v", c.bDeg, err)
		}

		if fromSouth := got == -1; fromSouth != c.south {
			t.Errorf("b = %g came from the %s file", c.bDeg,
				map[bool]string{true: "south", false: "north"}[fromSouth])
		}
	}
}

// An unopened map says so rather than dereferencing nothing.
func TestSFDRefusesBeforeOpen(t *testing.T) {
	t.Parallel()

	var s *SFD

	if _, err := s.IntensityAt(angle.Deg(0), angle.Deg(0)); err == nil {
		t.Error("a nil map answered")
	}

	if _, err := (&SFD{}).IntensityAt(angle.Deg(0), angle.Deg(0)); err == nil {
		t.Error("an empty map answered")
	}
}
