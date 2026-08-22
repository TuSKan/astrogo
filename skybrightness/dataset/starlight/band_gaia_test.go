package starlight_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
	"github.com/TuSKan/astrogo/unit"
)

// bessellVLike is a tophat over Johnson V carrying the Vega zero point SVO
// publishes for Generic/Bessell.V.
//
// A tophat rather than the real curve, so this file needs no network. Its pivot
// lands about three nanometres off the real one, which is why the comparison
// below allows two per cent rather than asserting equality.
func bessellVLike() magnitude.Passband {
	return magnitude.Passband{
		Name:            "Bessell V (tophat)",
		WavelengthNM:    []unit.WavelengthNM{506, 507, 598, 599},
		Response:        []float64{0, 1, 1, 0},
		Detector:        magnitude.EnergyIntegrating,
		VegaZeroPointJy: 3630.2172842325,
	}
}

// The zero point derived from a passband reproduces the one this package
// carried as a literal.
//
// The literal is 3.63e-11 W m^-2 nm^-1, written down for Johnson V. Deriving it
// instead from SVO's published 3630.22 Jy and the band's own pivot wavelength
// has to land on the same number, or one of the two is wrong and every
// magnitude in a map built on it is off by the difference.
func TestVegaZeroFluxReproducesTheShippedVLiteral(t *testing.T) {
	t.Parallel()

	got, err := starlight.VegaZeroFlux(bessellVLike())
	if err != nil {
		t.Fatalf("VegaZeroFlux: %v", err)
	}

	const shipped = 3.63e-11

	if rel := math.Abs(got-shipped) / shipped; rel > 0.02 {
		t.Errorf("derived %.4e against the shipped literal %.4e, %.1f per cent apart",
			got, shipped, 100*rel)
	}

	// The conversion itself, computed here from the band's own pivot so that a
	// mistyped power of ten fails even if the tophat's pivot drifts.
	pivot, err := bessellVLike().PivotWavelength()
	if err != nil {
		t.Fatalf("PivotWavelength: %v", err)
	}

	lambdaM := float64(pivot) * 1e-9
	want := 3630.2172842325 * 1e-26 * 2.99792458e8 / (lambdaM * lambdaM) * 1e-9

	if rel := math.Abs(got-want) / want; rel > 1e-9 {
		t.Errorf("F_lambda = %.6e, want %.6e from F_nu * c / pivot^2", got, want)
	}
}

// A passband with no zero point cannot produce a map band.
func TestVegaZeroFluxNeedsAZeroPoint(t *testing.T) {
	t.Parallel()

	b := bessellVLike()
	b.VegaZeroPointJy = 0

	if _, err := starlight.VegaZeroFlux(b); !errors.Is(err, starlight.ErrGaiaBand) {
		t.Errorf("err = %v, want ErrGaiaBand", err)
	}
}

// The four published relations resolve and U does not.
//
// Gaia's bluest band starts around 330 nm and Table 5.9 publishes no G-to-U
// relation, so a four-band map is what this catalogue can produce. Returning
// an error rather than an approximation is the whole point: a fabricated U
// would be indistinguishable from a real one in the output.
func TestJohnsonCousinsColourTermHasNoU(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"B", "V", "R", "I"} {
		got, err := starlight.JohnsonCousinsColourTerm(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}

		if len(got) < 3 {
			t.Errorf("%s: %d coefficients, too few for any of the published relations",
				name, len(got))
		}
	}

	for _, name := range []string{"U", "u", "g", "K", ""} {
		if _, err := starlight.JohnsonCousinsColourTerm(name); !errors.Is(err, starlight.ErrGaiaBand) {
			t.Errorf("%q: err = %v, want ErrGaiaBand", name, err)
		}
	}
}

// The general constructor reproduces the V band the package already had.
//
// GaiaJohnsonCousins("V", ...) replaced a dedicated GaiaJohnsonV, and the
// replacement is only safe if it produces the same band: the same colour
// polynomial and a flux-to-radiance factor within the tophat's own error of
// the one built from the literal zero point.
func TestGaiaJohnsonCousinsReproducesTheVBand(t *testing.T) {
	t.Parallel()

	got, err := starlight.GaiaJohnsonCousins("V", bessellVLike())
	if err != nil {
		t.Fatalf("GaiaJohnsonCousins: %v", err)
	}

	if got.Name != "V" {
		t.Errorf("name %q, want V", got.Name)
	}

	want := []float64{-0.02704, 0.01424, -0.2156, 0.01426}
	if len(got.ColourTerm) != len(want) {
		t.Fatalf("%d colour coefficients, want %d", len(got.ColourTerm), len(want))
	}

	for i := range want {
		if got.ColourTerm[i] != want[i] {
			t.Errorf("colour coefficient %d is %v, want %v", i, got.ColourTerm[i], want[i])
		}
	}

	// The V band built from the shipped literal, for comparison.
	const (
		shippedZeroFlux = 3.63e-11
		gZeroPoint      = 25.6874
	)

	reference := shippedZeroFlux / math.Pow(10, gZeroPoint/2.5)

	if rel := math.Abs(got.FluxToRadiance-reference) / reference; rel > 0.02 {
		t.Errorf("flux-to-radiance %.6e against the literal-built %.6e, %.1f per cent apart",
			got.FluxToRadiance, reference, 100*rel)
	}
}

// Every band resolves, and their zero points fall in the published order.
//
// Vega's spectrum falls toward the red across the optical, so the zero-point
// flux density is largest in B and smallest in I. A band wired to the wrong
// zero point shows here rather than as a plausible map.
func TestGaiaJohnsonCousinsOrdersTheZeroPoints(t *testing.T) {
	t.Parallel()

	// SVO's published Vega zero points for the Generic/Bessell curves, with
	// tophats spanning roughly each band.
	bands := []struct {
		name   string
		lo, hi unit.WavelengthNM
		jy     float64
	}{
		{"B", 380, 500, 3908.46},
		{"V", 507, 598, 3630.22},
		{"R", 570, 730, 3056.93},
		{"I", 720, 880, 2415.65},
	}

	var previous float64

	for i, b := range bands {
		p := magnitude.Passband{
			Name:            b.name,
			WavelengthNM:    []unit.WavelengthNM{b.lo - 1, b.lo, b.hi, b.hi + 1},
			Response:        []float64{0, 1, 1, 0},
			Detector:        magnitude.EnergyIntegrating,
			VegaZeroPointJy: b.jy,
		}

		got, err := starlight.GaiaJohnsonCousins(b.name, p)
		if err != nil {
			t.Fatalf("%s: %v", b.name, err)
		}

		if got.FluxToRadiance <= 0 {
			t.Errorf("%s: flux-to-radiance %g", b.name, got.FluxToRadiance)
		}

		if i > 0 && got.FluxToRadiance >= previous {
			t.Errorf("%s has zero-point flux %.4e against the bluer band's %.4e; Vega's "+
				"spectral flux density falls toward the red across the optical",
				b.name, got.FluxToRadiance, previous)
		}

		previous = got.FluxToRadiance
	}
}
