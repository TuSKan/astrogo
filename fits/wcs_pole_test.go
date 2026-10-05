package fits

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
)

// polarWCS is a 4096 × 3072 frame at 1.5″ per pixel, centered on
// (crval1, crval2) and projected with proj: the shape of a polar-alignment
// or celestial-pole image.
func polarWCS(t *testing.T, proj string, crval1, crval2 float64) *WCS {
	t.Helper()

	h := NewHeader()
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "CTYPE1", Value: "'RA---" + proj + "'"})
	h.Append(Card{Keyword: "CTYPE2", Value: "'DEC--" + proj + "'"})
	h.Append(Card{Keyword: "CRVAL1", Value: fmt.Sprintf("%.10f", crval1)})
	h.Append(Card{Keyword: "CRVAL2", Value: fmt.Sprintf("%.10f", crval2)})
	h.Append(Card{Keyword: "CRPIX1", Value: "2048.5"})
	h.Append(Card{Keyword: "CRPIX2", Value: "1536.5"})
	h.Append(Card{Keyword: "CDELT1", Value: "-0.000416667"})
	h.Append(Card{Keyword: "CDELT2", Value: "0.000416667"})

	w, err := ExtractWCS(h)
	if err != nil {
		t.Fatalf("ExtractWCS: %v", err)
	}

	return w
}

// TestWorldToPixelNearAPole maps pixels inside a frame near a celestial pole
// to the sky and back.
//
// WorldToPixel's analytic first guess read its axes transposed, projecting the
// declination as a right ascension about a transposed reference. The Newton
// loop after it recovered almost everywhere, but not here: 11% of these round
// trips failed at a center of 89°, 45% at 89.5° and 80% at 89.9°, with "did
// not converge" or "matrix is singular".
func TestWorldToPixelNearAPole(t *testing.T) {
	t.Parallel()

	for _, crval2 := range []float64{89, 89.5, 89.9, -89.9} {
		for _, crval1 := range []float64{3.7, 133.7, 273.7} {
			w := polarWCS(t, "TAN", crval1, crval2)

			for _, want := range [][]float64{{100, 100}, {1000, 2900}, {2048.5, 1536.5}, {3900, 300}, {4000, 3000}} {
				sky, err := w.PixelToWorld(want)
				if err != nil {
					t.Fatalf("PixelToWorld(%v): %v", want, err)
				}

				got, err := w.WorldToPixel(sky)
				if err != nil {
					t.Errorf("center (%v, %v): WorldToPixel(%v) for pixel %v: %v", crval1, crval2, sky, want, err)

					continue
				}

				if d := math.Hypot(got[0]-want[0], got[1]-want[1]); d > 1e-6 {
					t.Errorf("center (%v, %v): pixel %v came back as %v, %.3g px away", crval1, crval2, want, got, d)
				}
			}
		}
	}
}

// TestThePoleMapsToAPixelAndBack asks each zenithal projection where the
// celestial pole is in a frame that contains it, then asks what is at that
// pixel. It is the question a polar-alignment tool asks, and it failed three
// ways: the transposed first guess above; a declination computed with asin,
// which at the pole is 3 mas short or NaN; and a convergence test on the
// right ascension residual, which at the pole is undefined and never passes.
func TestThePoleMapsToAPixelAndBack(t *testing.T) {
	t.Parallel()

	for _, proj := range []string{"TAN", "SIN", "ARC", "STG"} {
		for i := range 200 {
			crval1 := float64(i%36)*10 + 0.37
			crval2 := 89.0 + float64(i%97)/100

			pole := 90.0
			if i%2 == 1 {
				crval2, pole = -crval2, -90.0
			}

			w := polarWCS(t, proj, crval1, crval2)

			px, err := w.WorldToPixel([]float64{crval1, pole})
			if err != nil {
				t.Errorf("%s center (%v, %v): WorldToPixel(pole): %v", proj, crval1, crval2, err)

				continue
			}

			sky, err := w.PixelToWorld(px)
			if err != nil {
				t.Fatalf("%s: PixelToWorld(%v): %v", proj, px, err)
			}

			if math.Abs(sky[1]-pole) > 1e-12 {
				t.Errorf("%s center (%v, %v): the pole's pixel %v is at declination %.15g, want %v",
					proj, crval1, crval2, px, sky[1], pole)
			}
		}
	}
}

// TestDeprojectIsFiniteAtThePole aims points at a celestial pole, exactly and
// within 1e-12 rad, through every projection. With the declination taken as
// asin of its sine, about one in ten came back NaN through TAN, ARC and STG.
func TestDeprojectIsFiniteAtThePole(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(1, 2))

	for _, proj := range []string{"TAN", "SIN", "ARC", "STG", "AIT"} {
		for range 100_000 {
			d0 := (rng.Float64()*2 - 1) * math.Pi / 2
			if rng.IntN(2) == 0 {
				d0 = math.Copysign(math.Pi/2-rng.Float64()*1e-3, d0)
			}

			// The pole's distance from the reference: north along +y, or
			// south along -y.
			north := rng.IntN(2) == 0

			dist := math.Pi/2 - d0
			if !north {
				dist = math.Pi/2 + d0
			}

			var x, y float64

			switch proj {
			case "TAN":
				y = math.Tan(dist)
			case "SIN":
				y = math.Sin(dist)
			case "ARC":
				y = dist
			case "STG":
				y = 2 * math.Tan(dist/2)
			case "AIT":
				x, y = rng.Float64()*4-2, rng.Float64()*2-1
			}

			if !north {
				y = -y
			}

			x += (rng.Float64() - 0.5) * 1e-12 * float64(rng.IntN(2))

			ra, dec, err := deproject(proj, x, y, 0, d0)
			if err != nil {
				continue // outside the projection's domain, which is an answer
			}

			if math.IsNaN(dec) || math.IsNaN(ra) || math.Abs(dec) > math.Pi/2 {
				t.Fatalf("%s: deproject(%.17g, %.17g, 0, %.17g) = (%v, %v)", proj, x, y, d0, ra, dec)
			}
		}
	}
}
