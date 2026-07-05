package skybrightness

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestGrid2DCorners verifies bilinear interpolation reproduces stored cell
// values exactly at the breakpoints, and that a cell midpoint is bounded by its
// four corners.
func TestGrid2DCorners(t *testing.T) {
	t.Parallel()

	// Exact breakpoints reproduce the stored values.
	for _, tc := range []struct{ i, j int }{{0, 0}, {3, 2}, {6, 9}, {18, 10}, {12, 5}} {
		got := zodiTable.at(zodiLon[tc.i], zodiLat[tc.j])
		testutil.AssertNear(t, "corner", got, zodiValues[tc.i][tc.j], 1e-9)
	}

	// Midpoint of a cell is bounded by its four corner values.
	lon := (zodiLon[6] + zodiLon[7]) / 2
	lat := (zodiLat[3] + zodiLat[4]) / 2
	mid := zodiTable.at(lon, lat)

	lo := math.Min(math.Min(zodiValues[6][3], zodiValues[6][4]), math.Min(zodiValues[7][3], zodiValues[7][4]))
	hi := math.Max(math.Max(zodiValues[6][3], zodiValues[6][4]), math.Max(zodiValues[7][3], zodiValues[7][4]))

	if mid < lo || mid > hi {
		t.Errorf("midpoint %.1f not bounded by neighbors [%.1f, %.1f]", mid, lo, hi)
	}
}

// TestGrid2DClamp verifies out-of-range queries clamp to the edge cells.
func TestGrid2DClamp(t *testing.T) {
	t.Parallel()

	loEdge := zodiTable.at(-50, -10)
	testutil.AssertNear(t, "clamp low", loEdge, zodiValues[0][0], 1e-9)

	hiEdge := zodiTable.at(500, 200)
	testutil.AssertNear(t, "clamp high", hiEdge, zodiValues[18][10], 1e-9)
}

// TestZodiPoleConversion cross-checks the SI → V conversion against the known
// dark-sky zodiacal minimum: the ecliptic pole (77 in Table-17 SI units = 60
// S10) is ~23.3 V mag/arcsec².
func TestZodiPoleConversion(t *testing.T) {
	t.Parallel()

	if got := zodiTable.at(120, 90); got != 77 {
		t.Errorf("pole cell: got %g, want 77 (10⁻⁸ W m⁻² sr⁻¹ µm⁻¹)", got)
	}

	sb := float64(siToSurfaceBrightnessV(77))
	testutil.AssertNear(t, "pole V mag/arcsec²", sb, 23.33, 0.1)
}

// TestZodiLatitudeMonotonic verifies brightness decreases with ecliptic latitude
// at fixed helioecliptic longitude (λ=30°), over the well-behaved range [0,75]°.
func TestZodiLatitudeMonotonic(t *testing.T) {
	t.Parallel()

	prev := math.Inf(1)

	for _, lat := range []float64{0, 5, 10, 15, 20, 25, 30, 45, 60, 75} {
		v := zodiTable.at(30, lat)
		if v > prev {
			t.Errorf("zodiacal not decreasing with latitude at λ=30°: lat=%g gives %.1f > previous %.1f", lat, v, prev)
		}

		prev = v
	}
}

// TestHelioLongitude verifies the |λ−λ☉| folding into [0,180]°.
func TestHelioLongitude(t *testing.T) {
	t.Parallel()

	cases := []struct{ lon, sun, want float64 }{
		{10, 350, 20},
		{200, 10, 170},
		{0, 0, 0},
		{180, 0, 180},
		{90, 270, 180},
	}
	for _, c := range cases {
		if got := helioLongitude(c.lon, c.sun); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("helioLongitude(%g,%g)=%g, want %g", c.lon, c.sun, got, c.want)
		}
	}
}

// TestZodiacalRadiance is an integration sanity check: a real evaluation yields
// a positive radiance with a plausible V surface brightness.
func TestZodiacalRadiance(t *testing.T) {
	t.Parallel()

	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	tm := time.FromJD(2451545.5, time.UTC)
	ctx := coord.NewContext(tm, loc, atmosphere.AtAltitude(0))

	z := NewZodiacalLight(nil)

	r, err := z.Radiance(coord.NewAltAz(angle.Deg(60), angle.Deg(120)), ctx)
	if err != nil {
		t.Fatalf("Radiance: %v", err)
	}

	if r <= 0 {
		t.Fatalf("expected positive radiance, got %g", r)
	}

	sb := float64(r.SurfaceBrightnessV())
	if sb < 18 || sb > 24 {
		t.Errorf("zodiacal V surface brightness %.2f outside plausible [18,24]", sb)
	}
}
