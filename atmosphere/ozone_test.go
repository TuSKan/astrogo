package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// TestOzoneAirmassIsAThinShell20kmUp holds OzoneAirmass to the geometry it
// stands for. A line of sight leaving the ground at zenith distance z meets a
// thin shell H = 20 km above an Earth of R = 6378 km at zenith distance z',
// with sin z' = R sin z / (R + H) by the law of sines in the triangle of the
// Earth's center, the observer and that point. The path through a thin layer
// is its thickness over cos z', so the airmass is 1/cos z': 12.66 at the
// horizon, where the molecular airmass is 38.75.
func TestOzoneAirmassIsAThinShell20kmUp(t *testing.T) {
	t.Parallel()

	const earthKM, shellKM = 6378.0, 20.0

	for _, alt := range []float64{0, 1, 5, 10, 30, 60, 90} {
		z := (90 - alt) * math.Pi / 180
		zShell := math.Asin(earthKM * math.Sin(z) / (earthKM + shellKM))
		want := 1 / math.Cos(zShell)

		got, err := atmosphere.OzoneAirmass(angle.Deg(alt))
		if err != nil {
			t.Fatalf("OzoneAirmass(%g°): %v", alt, err)
		}

		if math.Abs(got-want) > 1e-12*want {
			t.Errorf("OzoneAirmass(%g°) = %.12f, want %.12f", alt, got, want)
		}
	}

	if horizon, _ := atmosphere.OzoneAirmass(angle.Zero()); math.Abs(horizon-12.66) > 0.005 {
		t.Errorf("OzoneAirmass(0°) = %.4f, want 12.66", horizon)
	}

	if _, err := atmosphere.OzoneAirmass(angle.Deg(-1)); !errors.Is(err, atmosphere.ErrBelowHorizon) {
		t.Errorf("OzoneAirmass(-1°) error = %v, want ErrBelowHorizon", err)
	}
}

// TestOzoneOpticalDepthIsTheColumnTimesTheCrossSection checks the vertical
// optical depth against the cross section the table prints at four nodes, the
// column, and the Dobson unit: 0.0349 at 600 nm for 258 DU, the figure #632
// quotes. It is also exactly the ozone term of Extinction, the share of the
// coefficient that adding the column adds.
func TestOzoneOpticalDepthIsTheColumnTimesTheCrossSection(t *testing.T) {
	t.Parallel()

	const column = 258

	air, err := atmosphere.NewBuilder().Surface(1013.25, 288.15).Ozone(column).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	none, err := atmosphere.NewBuilder().Surface(1013.25, 288.15).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, node := range []struct {
		nm    unit.WavelengthNM
		sigma float64 // cm^2 per molecule, at 223 K
	}{{500, 1.1791e-21}, {550, 3.2853e-21}, {600, 5.0348e-21}, {650, 2.4440e-21}} {
		want := node.sigma * column * atmosphere.DobsonUnitMoleculesPerCM2()

		got, err := air.OzoneOpticalDepth(node.nm)
		if err != nil {
			t.Fatalf("OzoneOpticalDepth(%v): %v", node.nm, err)
		}

		if math.Abs(float64(got)-want) > 1e-12*want {
			t.Errorf("OzoneOpticalDepth(%v) = %.8f, want %.8f", node.nm, float64(got), want)
		}

		with, err := air.Extinction(node.nm)
		if err != nil {
			t.Fatalf("Extinction(%v): %v", node.nm, err)
		}

		without, err := none.Extinction(node.nm)
		if err != nil {
			t.Fatalf("Extinction(%v) without ozone: %v", node.nm, err)
		}

		if term := (with - without) / (2.5 * math.Log10(math.E)); math.Abs(term-float64(got)) > 1e-12 {
			t.Errorf("at %v the column adds %.10f to Extinction's optical depth, OzoneOpticalDepth says %.10f",
				node.nm, term, float64(got))
		}
	}

	if got, _ := air.OzoneOpticalDepth(600); math.Abs(float64(got)-0.0349) > 0.00005 {
		t.Errorf("OzoneOpticalDepth(600 nm) for 258 DU = %.5f, want 0.0349", float64(got))
	}
}

// TestOzoneOpticalDepthOutsideTheTable: beyond 320 to 1000 nm the cross
// section is not tabulated, so a column there is refused rather than given an
// invented depth. No column is zero depth anywhere, since nothing absorbs;
// that keeps an ozone-free scene working on any grid.
func TestOzoneOpticalDepthOutsideTheTable(t *testing.T) {
	t.Parallel()

	air, err := atmosphere.NewBuilder().Ozone(300).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	none, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, nm := range []unit.WavelengthNM{300, 1200} {
		if _, err := air.OzoneOpticalDepth(nm); !errors.Is(err, atmosphere.ErrExtinctionWavelength) {
			t.Errorf("OzoneOpticalDepth(%v) with a column: error = %v, want ErrExtinctionWavelength", nm, err)
		}

		if got, err := none.OzoneOpticalDepth(nm); err != nil || got != 0 {
			t.Errorf("OzoneOpticalDepth(%v) with no column = %v, %v; want 0, nil", nm, got, err)
		}
	}
}
