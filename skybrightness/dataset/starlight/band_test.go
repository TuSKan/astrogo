package starlight_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// Band is the adapter the engine consumes, so what it hands back must agree
// with the map it came from, pixel for pixel.
func TestBandMatchesTheMap(t *testing.T) {
	t.Parallel()

	m, err := starlight.Load(strings.NewReader(tableFor("V")), starlight.Galactic)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	view, err := m.Band("V")
	if err != nil {
		t.Fatalf("Band: %v", err)
	}

	for pixel := range m.Grid().NumPixels() {
		lon, lat, err := m.Grid().Center(pixel)
		if err != nil {
			t.Fatalf("Center(%d): %v", pixel, err)
		}

		want, err := m.RadianceAt("V", lon, lat)
		if err != nil {
			t.Fatalf("RadianceAt: %v", err)
		}

		got, err := view.RadianceAt(lon, lat)
		if err != nil {
			t.Fatalf("Band RadianceAt: %v", err)
		}

		if got != want {
			t.Fatalf("pixel %d: band view gave %v, map gave %v", pixel, got, want)
		}
	}
}

// The frame has to travel with the data. A galactic map read as equatorial
// puts the Milky Way through the wrong part of the sky and still returns
// plausible numbers everywhere, so the adapter reports the map's own frame
// rather than letting the engine assume one.
func TestBandCarriesTheFrame(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		frame starlight.Frame
		want  bool
	}{
		{starlight.Galactic, true},
		{starlight.ICRS, false},
	} {
		m, err := starlight.Load(strings.NewReader(tableFor("V")), tc.frame)
		if err != nil {
			t.Fatalf("Load(%v): %v", tc.frame, err)
		}

		view, err := m.Band("V")
		if err != nil {
			t.Fatalf("Band(%v): %v", tc.frame, err)
		}

		if got := view.Galactic(); got != tc.want {
			t.Errorf("frame %v: Galactic() = %v, want %v", tc.frame, got, tc.want)
		}
	}
}

// Asking for a band the map does not hold is an error rather than an empty
// view that would read as a dark sky everywhere.
func TestBandRejectsUnknownName(t *testing.T) {
	t.Parallel()

	m, err := starlight.Load(strings.NewReader(tableFor("V")), starlight.Galactic)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if _, err := m.Band("R"); !errors.Is(err, starlight.ErrBand) {
		t.Errorf("err = %v, want ErrBand", err)
	}
}

// The three published numbers behind the Gaia G to Johnson V conversion, each
// checked against its source, because the failure mode of getting one wrong is
// a map that is plausible everywhere.
func TestGaiaJohnsonV(t *testing.T) {
	t.Parallel()

	band := starlight.GaiaJohnsonV()

	if band.Name != "V" {
		t.Errorf("Name = %q, want V", band.Name)
	}

	// The colour term is Riello et al. (2021) Table 5.7 negated: the table
	// prints V - G, and the query's +0.4 exponent needs G - V.
	want := []float64{0.02704, -0.01424, 0.2156, -0.01426}
	if len(band.ColourTerm) != len(want) {
		t.Fatalf("ColourTerm has %d terms, want %d", len(band.ColourTerm), len(want))
	}

	for i, c := range want {
		if band.ColourTerm[i] != c {
			t.Errorf("ColourTerm[%d] = %v, want %v", i, band.ColourTerm[i], c)
		}
	}

	// A G=0, Vega-coloured source: BP-RP = 0 makes the polynomial its constant
	// term, and the flux is 10^(25.6874/2.5) e-/s by the definition of the
	// zero point. What comes out must be Johnson V's own Vega zero point,
	// 3.63e-11 W m^-2 nm^-1, shifted by that constant.
	flux := math.Pow(10, 25.6874/2.5)
	got := flux * band.FluxToRadiance * math.Pow(10, 0.4*band.ColourTerm[0])

	if wantFlux := 3.63e-11 * math.Pow(10, 0.4*0.02704); math.Abs(got-wantFlux)/wantFlux > 1e-12 {
		t.Errorf("a Vega-coloured G=0 source gives %.6e, want %.6e", got, wantFlux)
	}

	// The band must be usable as it stands — this is the whole point of it
	// being a constructor rather than three numbers at the call site.
	if _, err := (starlight.GaiaBuild{FainterThan: starlight.NoMagnitudeCut, Bands: []starlight.GaiaBand{band}}).ADQL(0, 9); err != nil {
		t.Errorf("ADQL: %v", err)
	}
}

// A red star is fainter in G than in V, so it must gain flux in the V map.
// This is the sign of the colour term, and getting it backwards is a mistake
// that shows up only along the Galactic plane.
func TestGaiaJohnsonVBrightensRedStars(t *testing.T) {
	t.Parallel()

	term := starlight.GaiaJohnsonV().ColourTerm

	gMinusV := func(c float64) float64 {
		return term[0] + term[1]*c + term[2]*c*c + term[3]*c*c*c
	}

	// A solar-colour star, BP-RP around 0.82, is close to neutral; an M dwarf
	// at BP-RP = 3 is well into the red.
	if red := gMinusV(3); red <= gMinusV(0.82) {
		t.Errorf("G-V at BP-RP=3 is %v, must exceed the solar-colour value %v", red, gMinusV(0.82))
	}

	if gMinusV(3) <= 0 {
		t.Errorf("a red star must be brighter in V than in G; G-V = %v", gMinusV(3))
	}
}
