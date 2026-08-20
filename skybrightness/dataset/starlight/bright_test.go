package starlight

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// The Gaia path and the Hipparcos path must agree on the same star exactly,
// not approximately.
//
// One converts a Gaia flux through a colour term and two zero points; the other
// reads Johnson V straight from a catalogue. They meet in the same map, so any
// disagreement is a seam nothing downstream could see — the map would be wrong
// by the difference wherever the two overlap, with every value still positive
// and plausible.
//
// The agreement is exact by construction, and the algebra is worth stating
// because it is what the tolerance below rests on. Writing ZP for Gaia's G
// zero point and c for the G - V offset:
//
//	F_G      = 10^(0.4(ZP - G))                 by definition of the zero point
//	radiance = F_G * 10^(0.4(G - V)) * zp * 10^(-0.4*ZP)
//	         = zp * 10^(0.4(ZP - G + G - V - ZP))
//	         = zp * 10^(-0.4V)
//
// Every term involving ZP, G and c cancels, leaving exactly what the Hipparcos
// path computes from V alone. So the tolerance is floating-point noise, not a
// physical allowance — anything larger means a real divergence.
func TestBothPathsAgreeOnTheSameStar(t *testing.T) {
	t.Parallel()

	band := GaiaJohnsonV()

	// A Vega-coloured star: BP-RP = 0 collapses the colour polynomial to its
	// constant term c, so G = V + c and the flux producing that G follows from
	// the zero point.
	c := band.ColourTerm[0]

	for _, v := range []float64{-1.44, 0.03, 4.0, 11.5} {
		g := v + c
		flux := math.Pow(10, 0.4*(25.6874-g))

		viaGaia := flux * band.FluxToRadiance * math.Pow(10, 0.4*c)
		viaHipparcos := johnsonVZeroFlux * math.Pow(10, -0.4*v)

		if rel := math.Abs(viaGaia-viaHipparcos) / viaHipparcos; rel > 1e-12 {
			t.Errorf("V = %.2f: Gaia path %.9e, Hipparcos path %.9e, %.2e apart",
				v, viaGaia, viaHipparcos, rel)
		}
	}
}

// A star's light lands in its own pixel, scaled by that pixel's solid angle.
func TestAddBrightStarsPlacesAndScales(t *testing.T) {
	t.Parallel()

	m, err := NewMap(ICRS, map[string][]float64{"V": make([]float64, 3072)})
	if err != nil {
		t.Fatalf("NewMap: %v", err)
	}

	star := BrightStar{HIP: 32349, RA: angle.Deg(101.2885), Dec: angle.Deg(-16.7131), Vmag: -1.44}

	if err := AddBrightStars(m, "V", []BrightStar{star}); err != nil {
		t.Fatalf("AddBrightStars: %v", err)
	}

	grid := m.Grid()
	pixel := pixelOfStar(grid, star)
	solidAngle := 4 * math.Pi / float64(grid.NumPixels())
	want := johnsonVZeroFlux * math.Pow(10, -0.4*star.Vmag) / solidAngle

	got, err := m.Pixel("V", pixel)
	if err != nil {
		t.Fatalf("Pixel: %v", err)
	}

	if math.Abs(got-want)/want > 1e-12 {
		t.Errorf("pixel %d = %.6e, want %.6e", pixel, got, want)
	}

	// Every other pixel is untouched: a star is a point, not a background.
	var lit int

	for p := range grid.NumPixels() {
		if v, _ := m.Pixel("V", p); v > 0 {
			lit++
		}
	}

	if lit != 1 {
		t.Errorf("%d pixels lit, want 1", lit)
	}
}

// Adding is adding: a star sums into whatever the Gaia aggregation already
// left there rather than replacing it.
func TestAddBrightStarsAccumulates(t *testing.T) {
	t.Parallel()

	values := make([]float64, 3072)
	for i := range values {
		values[i] = 1e-9
	}

	m, err := NewMap(ICRS, map[string][]float64{"V": values})
	if err != nil {
		t.Fatalf("NewMap: %v", err)
	}

	star := BrightStar{HIP: 91262, RA: angle.Deg(279.234), Dec: angle.Deg(38.784), Vmag: 0.03}

	if err := AddBrightStars(m, "V", []BrightStar{star}); err != nil {
		t.Fatalf("AddBrightStars: %v", err)
	}

	pixel := pixelOfStar(m.Grid(), star)

	got, _ := m.Pixel("V", pixel)
	if got <= 1e-9 {
		t.Errorf("pixel %d = %v, which did not accumulate onto the existing 1e-9", pixel, got)
	}
}

// The brightest objects in the sky are not rows to drop silently.
func TestAddBrightStarsRejectsUnusableInput(t *testing.T) {
	t.Parallel()

	m, err := NewMap(ICRS, map[string][]float64{"V": make([]float64, 3072)})
	if err != nil {
		t.Fatalf("NewMap: %v", err)
	}

	if err := AddBrightStars(m, "R", nil); !errors.Is(err, ErrBand) {
		t.Errorf("unknown band: err = %v, want ErrBand", err)
	}

	bad := []BrightStar{{HIP: 1, Vmag: math.NaN()}}
	if err := AddBrightStars(m, "V", bad); !errors.Is(err, ErrBrightStar) {
		t.Errorf("NaN magnitude: err = %v, want ErrBrightStar", err)
	}

	if err := AddBrightStars(nil, "V", nil); !errors.Is(err, ErrBrightStar) {
		t.Errorf("nil map: err = %v, want ErrBrightStar", err)
	}
}

// Hipparcos is catalogued at J1991.25 and Gaia DR3 at J2016.0. Matching
// without propagating that quarter-century of proper motion misses genuine
// counterparts for the nearby fast movers — which are also, unhelpfully, the
// bright ones — and every miss becomes a star added to the map twice.
func TestBrightStarsMissingFromGaiaPropagatesProperMotion(t *testing.T) {
	t.Parallel()

	// A star at the origin moving 1 arcsecond a year in declination. Over
	// 24.75 years it moves 24.75", far outside a 5" match radius.
	star := BrightStar{HIP: 1, RA: angle.Deg(0), Dec: angle.Deg(0), Vmag: 2}

	// Gaia sees it where it now is, not where Hipparcos recorded it.
	gaiaRA := []angle.Angle{angle.Deg(0)}
	gaiaDec := []angle.Angle{angle.Deg(24.75 / 3600)}

	pmDec := []float64{1000} // mas/yr = 1"/yr

	missing, err := BrightStarsMissingFromGaia(
		[]BrightStar{star}, []float64{0}, pmDec, gaiaRA, gaiaDec, angle.Deg(5.0/3600))
	if err != nil {
		t.Fatalf("BrightStarsMissingFromGaia: %v", err)
	}

	if len(missing) != 0 {
		t.Error("the star moved onto its Gaia counterpart and must match, not be re-added")
	}

	// Without the proper motion it would not have matched, which is the bug
	// this guards: the same call with no motion leaves it unmatched.
	stationary, err := BrightStarsMissingFromGaia(
		[]BrightStar{star}, []float64{0}, []float64{0}, gaiaRA, gaiaDec, angle.Deg(5.0/3600))
	if err != nil {
		t.Fatalf("BrightStarsMissingFromGaia: %v", err)
	}

	if len(stationary) != 1 {
		t.Error("without proper motion the star should not match, or the test proves nothing")
	}
}

// A star Gaia genuinely lacks is reported; one it has is not.
func TestBrightStarsMissingFromGaiaFindsTheGap(t *testing.T) {
	t.Parallel()

	seen := BrightStar{HIP: 1, RA: angle.Deg(10), Dec: angle.Deg(20), Vmag: 3}
	unseen := BrightStar{HIP: 2, RA: angle.Deg(200), Dec: angle.Deg(-40), Vmag: -1}

	missing, err := BrightStarsMissingFromGaia(
		[]BrightStar{seen, unseen},
		[]float64{0, 0}, []float64{0, 0},
		[]angle.Angle{angle.Deg(10)}, []angle.Angle{angle.Deg(20)},
		angle.Deg(5.0/3600))
	if err != nil {
		t.Fatalf("BrightStarsMissingFromGaia: %v", err)
	}

	if len(missing) != 1 || missing[0].HIP != 2 {
		t.Errorf("missing = %v, want only HIP 2", missing)
	}
}

func TestBrightStarsMissingFromGaiaValidates(t *testing.T) {
	t.Parallel()

	_, err := BrightStarsMissingFromGaia(
		[]BrightStar{{HIP: 1}}, nil, nil, nil, nil, angle.Deg(1))
	if !errors.Is(err, ErrBrightStar) {
		t.Errorf("mismatched proper motions: err = %v, want ErrBrightStar", err)
	}

	if err != nil && !strings.Contains(err.Error(), "proper motion") {
		t.Errorf("the error should say what mismatched: %v", err)
	}
}
