package coord_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

func geo(t *testing.T, lonDeg, latDeg float64) *coord.Geodetic {
	t.Helper()

	g, err := coord.NewGeodetic(angle.Deg(lonDeg), angle.Deg(latDeg), 0)
	if err != nil {
		t.Fatalf("NewGeodetic(%v, %v): %v", lonDeg, latDeg, err)
	}

	return g
}

// One degree of latitude is very close to 111.2 km on the mean sphere,
// which is an independently known figure rather than one derived from this
// implementation.
func TestGroundDistanceOneDegreeLatitude(t *testing.T) {
	t.Parallel()

	d, err := coord.GroundDistance(geo(t, 0, 0), geo(t, 0, 1))
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	const want = 111195.0 // metres, pi/180 * mean radius

	if rel := math.Abs(d-want) / want; rel > 1e-3 {
		t.Errorf("one degree of latitude = %.1f m, want ~%.0f m", d, want)
	}
}

// A quarter of the way around the Earth: pole to equator must be a quarter
// of the meridional circumference.
func TestGroundDistancePoleToEquator(t *testing.T) {
	t.Parallel()

	d, err := coord.GroundDistance(geo(t, 0, 90), geo(t, 0, 0))
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	want := math.Pi / 2 * 6371008.8

	if rel := math.Abs(d-want) / want; rel > 1e-9 {
		t.Errorf("pole to equator = %.1f m, want %.1f m", d, want)
	}
}

// The date line is where a naive longitude difference breaks: two points a
// degree apart across it must be a degree apart, not 359.
func TestGroundDistanceAcrossDateLine(t *testing.T) {
	t.Parallel()

	across, err := coord.GroundDistance(geo(t, 179.5, 0), geo(t, -179.5, 0))
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	same, err := coord.GroundDistance(geo(t, 0, 0), geo(t, 1, 0))
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	if rel := math.Abs(across-same) / same; rel > 1e-9 {
		t.Errorf("one degree across the date line = %.1f m, want the same as elsewhere %.1f m", across, same)
	}
}

// Short distances are where the spherical law of cosines loses precision
// and haversine does not. A metre-scale separation must come out as a
// metre-scale number rather than as noise.
func TestGroundDistanceShortBaseline(t *testing.T) {
	t.Parallel()

	// About 1.1 m of latitude.
	d, err := coord.GroundDistance(geo(t, 0, 0), geo(t, 0, 1e-5))
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	want := 1e-5 * math.Pi / 180 * 6371008.8

	if rel := math.Abs(d-want) / want; rel > 1e-6 {
		t.Errorf("short baseline = %.6f m, want %.6f m", d, want)
	}
}

func TestGroundDistanceIsSymmetricAndZero(t *testing.T) {
	t.Parallel()

	a, b := geo(t, 10, 45), geo(t, -70.4, -24.6)

	ab, err := coord.GroundDistance(a, b)
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	ba, err := coord.GroundDistance(b, a)
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	if math.Abs(ab-ba) > 1e-9 {
		t.Errorf("distance is not symmetric: %v vs %v", ab, ba)
	}

	self, err := coord.GroundDistance(a, a)
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	if self != 0 {
		t.Errorf("distance to self = %v, want 0", self)
	}
}

// Cardinal bearings are the clearest check that the formula measures
// clockwise from north rather than some other convention.
func TestInitialBearingCardinals(t *testing.T) {
	t.Parallel()

	origin := geo(t, 0, 0)

	cases := []struct {
		name string
		to   *coord.Geodetic
		want float64
	}{
		{"north", geo(t, 0, 1), 0},
		{"east", geo(t, 1, 0), 90},
		{"south", geo(t, 0, -1), 180},
		{"west", geo(t, -1, 0), 270},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := coord.InitialBearing(origin, tc.to)
			if err != nil {
				t.Fatalf("InitialBearing: %v", err)
			}

			if math.Abs(got.Degrees()-tc.want) > 1e-6 {
				t.Errorf("bearing = %v, want %v", got.Degrees(), tc.want)
			}
		})
	}
}

func TestGroundNilLocations(t *testing.T) {
	t.Parallel()

	if _, err := coord.GroundDistance(nil, geo(t, 0, 0)); !errors.Is(err, coord.ErrNilGeodetic) {
		t.Errorf("GroundDistance(nil, ...) = %v, want ErrNilGeodetic", err)
	}

	if _, err := coord.InitialBearing(geo(t, 0, 0), nil); !errors.Is(err, coord.ErrNilGeodetic) {
		t.Errorf("InitialBearing(..., nil) = %v, want ErrNilGeodetic", err)
	}
}

// Offset is the direct problem to GroundDistance and InitialBearing's
// inverse one, so a round trip has to return to where it started. That is
// the check that all three share one sphere and one angle convention.
func TestOffsetRoundTrip(t *testing.T) {
	t.Parallel()

	start := geo(t, -70.4, -24.6)

	for _, tc := range []struct {
		bearing  float64
		distance float64
	}{
		{0, 1000}, {90, 50_000}, {180, 250_000}, {270, 10}, {45, 1_000_000},
	} {
		end, err := coord.Offset(start, angle.Deg(tc.bearing), tc.distance)
		if err != nil {
			t.Fatalf("Offset(%v deg, %v m): %v", tc.bearing, tc.distance, err)
		}

		d, err := coord.GroundDistance(start, end)
		if err != nil {
			t.Fatalf("GroundDistance: %v", err)
		}

		if rel := math.Abs(d-tc.distance) / tc.distance; rel > 1e-9 {
			t.Errorf("bearing %v: travelled %.4f m, want %.4f", tc.bearing, d, tc.distance)
		}

		b, err := coord.InitialBearing(start, end)
		if err != nil {
			t.Fatalf("InitialBearing: %v", err)
		}

		if diff := math.Abs(b.Wrap2Pi().Degrees() - tc.bearing); diff > 1e-6 && math.Abs(diff-360) > 1e-6 {
			t.Errorf("bearing came back as %v, want %v", b.Wrap2Pi().Degrees(), tc.bearing)
		}
	}
}

// Due north from the equator is a pure latitude change, and due east is a
// pure longitude change — the two cases where the answer is obvious.
func TestOffsetCardinals(t *testing.T) {
	t.Parallel()

	origin := geo(t, 0, 0)

	north, err := coord.Offset(origin, angle.Deg(0), 111195)
	if err != nil {
		t.Fatalf("Offset: %v", err)
	}

	if math.Abs(north.Lat().Degrees()-1) > 1e-3 || math.Abs(north.Lon().Degrees()) > 1e-9 {
		t.Errorf("one degree north = (%v, %v), want (0, 1)", north.Lon().Degrees(), north.Lat().Degrees())
	}

	east, err := coord.Offset(origin, angle.Deg(90), 111195)
	if err != nil {
		t.Fatalf("Offset: %v", err)
	}

	if math.Abs(east.Lon().Degrees()-1) > 1e-3 || math.Abs(east.Lat().Degrees()) > 1e-9 {
		t.Errorf("one degree east = (%v, %v), want (1, 0)", east.Lon().Degrees(), east.Lat().Degrees())
	}
}

// Crossing the date line must wrap the longitude rather than run past 180.
func TestOffsetWrapsDateLine(t *testing.T) {
	t.Parallel()

	start := geo(t, 179.9, 0)

	end, err := coord.Offset(start, angle.Deg(90), 50_000)
	if err != nil {
		t.Fatalf("Offset: %v", err)
	}

	if lon := end.Lon().Degrees(); lon > 0 {
		t.Errorf("crossing the date line eastward gave longitude %v, want a negative value", lon)
	}

	d, err := coord.GroundDistance(start, end)
	if err != nil {
		t.Fatalf("GroundDistance: %v", err)
	}

	if math.Abs(d-50_000) > 1 {
		t.Errorf("distance across the date line = %v m, want 50000", d)
	}
}

func TestOffsetNil(t *testing.T) {
	t.Parallel()

	if _, err := coord.Offset(nil, 0, 100); !errors.Is(err, coord.ErrNilGeodetic) {
		t.Errorf("Offset(nil, ...) = %v, want ErrNilGeodetic", err)
	}
}
