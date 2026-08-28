package starlight

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// The sexagesimal fallback is not decoration: 262 Hipparcos entries carry no
// ICRS position because the astrometric fit failed on them, and three of those
// are naked-eye stars. An exploratory script silently lost all three.
func TestSexagesimalParsesBothHemispheres(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		isHours bool
		want    float64 // degrees
	}{
		{"00 00 00.00", true, 0},
		{"12 00 00.00", true, 180},
		{"06 30 00.00", true, 97.5},
		{"+38 51 33.4", false, 38.859278},
		{"-16 42 58.0", false, -16.716111},
		{"-00 30 00.0", false, -0.5}, // the sign lives on a zero degree field
	}

	for _, c := range cases {
		got, ok := sexagesimal(c.in, c.isHours)
		if !ok {
			t.Errorf("sexagesimal(%q) failed", c.in)

			continue
		}

		if math.Abs(got.Degrees()-c.want) > 1e-5 {
			t.Errorf("sexagesimal(%q) = %.6f deg, want %.6f", c.in, got.Degrees(), c.want)
		}
	}

	for _, bad := range []string{"", "12 00", "12 00 00 00", "aa bb cc"} {
		if _, ok := sexagesimal(bad, true); ok {
			t.Errorf("sexagesimal(%q) reported success", bad)
		}
	}
}

// A star with no ICRS position must still be placed, and one with no magnitude
// must be dropped rather than placed at zero.
func TestParseHipparcosRecoversPositionsAndDropsUnusableRows(t *testing.T) {
	t.Parallel()

	// HIP 3 has ICRS; 55203 has only sexagesimal, as in the real catalogue;
	// the last row has no magnitude at all.
	csv := "HIP,RAICRS,DEICRS,Vmag,pmRA,pmDE,RAhms,DEdms\n" +
		"3,0.00500795,38.85928608,6.61,5.24,-2.91,00 00 01.20,+38 51 33.4\n" +
		"55203,,,3.79,-430.31,-587.29,11 18 10.90,+31 31 45.0\n" +
		"99999,10.0,20.0,,1.0,1.0,00 40 00.00,+20 00 00.0\n"

	stars, pmRA, pmDec, err := parseHipparcos(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parseHipparcos: %v", err)
	}

	if len(stars) != 2 {
		t.Fatalf("got %d stars, want 2 (the row without a magnitude must be dropped)", len(stars))
	}

	if len(pmRA) != 2 || len(pmDec) != 2 {
		t.Fatalf("proper motions must stay aligned with stars: %d/%d", len(pmRA), len(pmDec))
	}

	if stars[0].HIP != 3 || math.Abs(stars[0].RA.Degrees()-0.005008) > 1e-5 {
		t.Errorf("ICRS row parsed as HIP %d at RA %.6f", stars[0].HIP, stars[0].RA.Degrees())
	}

	// 11h 18m 10.90s = 169.545 deg. Recovered only via the sexagesimal columns.
	if stars[1].HIP != 55203 {
		t.Fatalf("the star lacking an ICRS position was dropped")
	}

	if got := stars[1].RA.Degrees(); math.Abs(got-169.5454) > 1e-3 {
		t.Errorf("HIP 55203 RA = %.4f deg, want 169.5454 from the sexagesimal columns", got)
	}

	if got := stars[1].Dec.Degrees(); math.Abs(got-31.5292) > 1e-3 {
		t.Errorf("HIP 55203 Dec = %.4f deg, want 31.5292", got)
	}

	if stars[1].Vmag != 3.79 {
		t.Errorf("HIP 55203 V = %v, want 3.79", stars[1].Vmag)
	}
}

// An empty response is an error, not an empty sky. Silently returning no stars
// would mean the map ships without its brightest 6.4 per cent and nothing says so.
func TestParseHipparcosRejectsAnEmptyResult(t *testing.T) {
	t.Parallel()

	only := "HIP,RAICRS,DEICRS,Vmag,pmRA,pmDE,RAhms,DEdms\n"

	if _, _, _, err := parseHipparcos(strings.NewReader(only)); !errors.Is(err, ErrBrightStar) {
		t.Errorf("empty result: err = %v, want ErrBrightStar", err)
	}
}

// Column names differ between archives — ESA lowercases them, VizieR keeps the
// case it was given — so the reader indexes case-insensitively.
func TestReadTAPCSVIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	rows, index, err := readTAPCSV(strings.NewReader("RAICRS,Vmag\n1.5,2.5\n"))
	if err != nil {
		t.Fatalf("readTAPCSV: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}

	if v, ok := numField(index, rows[0], "raicrs"); !ok || v != 1.5 {
		t.Errorf("raicrs = %v, %v; want 1.5, true", v, ok)
	}

	if _, ok := numField(index, rows[0], "nosuchcolumn"); ok {
		t.Error("a missing column reported success")
	}
}

// The defaults are the measured ones, and the radius constant has to be the
// angle it claims to be — it is written as a constant expression rather than
// angle.Arcsec(5), so nothing else checks it.
func TestBrightStarDefaults(t *testing.T) {
	t.Parallel()

	if BrightStarLimitV != 7.0 {
		t.Errorf("BrightStarLimitV = %v, want 7", BrightStarLimitV)
	}

	if got := BrightStarMatchRadius.Arcseconds(); math.Abs(got-5) > 1e-9 {
		t.Errorf("BrightStarMatchRadius = %.9f arcsec, want 5", got)
	}
}

// Bad arguments fail before any request is made, so a typo does not become a
// query against somebody else's archive.
func TestFetchBrightStarsValidates(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name   string
		v      float64
		radius angle.Angle
	}{
		{"NaN limit", math.NaN(), BrightStarMatchRadius},
		{"absurd limit", -3, BrightStarMatchRadius},
		{"zero radius", BrightStarLimitV, 0},
		{"negative radius", BrightStarLimitV, -1},
	} {
		if _, err := FetchBrightStars(t.Context(), c.v, c.radius); !errors.Is(err, ErrBrightStar) {
			t.Errorf("%s: err = %v, want ErrBrightStar", c.name, err)
		}
	}
}
