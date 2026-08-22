package starlight

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// The archive returns its column names lowercased. The index is built that
// way, so the per-band lookup has to match — it did not, and every build
// failed with "no column for band" after the query had already succeeded.
//
// This is the regression test for that: a header in the archive's own casing,
// against a band named in the caller's.
func TestAccumulateMatchesLowercaseHeader(t *testing.T) {
	t.Parallel()

	build := GaiaBuild{Order: 4,
		Bands: []GaiaBand{{Name: "G", FluxToRadiance: 2.0}},
	}

	// Exactly what the archive sends: lowercase names, scientific notation.
	csv := "hpx,n,ncolour,b_g\n" +
		"0,500,480,1.0E3\n" +
		"1,600,590,2.0E3\n"

	bands := map[string][]float64{"G": make([]float64, 3072)}
	counts := make([]int64, 3072)

	const solidAngle = 4 * math.Pi / 3072

	rows, err := newCSVRows(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("newCSVRows: %v", err)
	}

	if err := build.accumulate(rows, bands, counts, solidAngle); err != nil {
		t.Fatalf("accumulate: %v", err)
	}

	// flux * FluxToRadiance / solid angle.
	for pixel, flux := range map[int64]float64{0: 1e3, 1: 2e3} {
		want := flux * 2.0 / solidAngle
		if got := bands["G"][pixel]; math.Abs(got-want)/want > 1e-12 {
			t.Errorf("pixel %d = %v, want %v", pixel, got, want)
		}
	}

	if counts[0] != 500 || counts[1] != 600 {
		t.Errorf("source counts = %v, %v, want 500, 600", counts[0], counts[1])
	}

	// A pixel the chunk did not report stays zero rather than becoming NaN.
	if bands["G"][2] != 0 {
		t.Errorf("unreported pixel is %v, want 0", bands["G"][2])
	}
}

// A band the response does not carry is a contract violation, not something
// to skip quietly — it means the query and the parser disagree.
func TestAccumulateRejectsMissingBand(t *testing.T) {
	t.Parallel()

	build := GaiaBuild{Order: 4, Bands: []GaiaBand{{Name: "V", FluxToRadiance: 1}}}

	err := build.accumulate(
		mustRows(t, "hpx,n,ncolour,b_g\n0,1,1,1.0\n"),
		map[string][]float64{"V": make([]float64, 3072)},
		make([]int64, 3072),
		1,
	)
	if !errors.Is(err, ErrGaiaResponse) {
		t.Errorf("err = %v, want ErrGaiaResponse", err)
	}
}

// A pixel index outside the map means the divisor and the map disagree, which
// would otherwise write past the end of a band or silently into the wrong sky.
func TestAccumulateRejectsPixelOutOfRange(t *testing.T) {
	t.Parallel()

	build := GaiaBuild{Order: 4, Bands: []GaiaBand{{Name: "G", FluxToRadiance: 1}}}

	for _, row := range []string{"99999,1,1,1.0", "-1,1,1,1.0", "abc,1,1,1.0"} {
		err := build.accumulate(
			mustRows(t, "hpx,n,ncolour,b_g\n"+row+"\n"),
			map[string][]float64{"G": make([]float64, 3072)},
			make([]int64, 3072),
			1,
		)
		if !errors.Is(err, ErrGaiaResponse) {
			t.Errorf("row %q: err = %v, want ErrGaiaResponse", row, err)
		}
	}
}

// A pixel whose flux the archive could not compute contributes nothing rather
// than poisoning the map with a NaN.
func TestAccumulateSkipsUnusableFlux(t *testing.T) {
	t.Parallel()

	build := GaiaBuild{Order: 4, Bands: []GaiaBand{{Name: "G", FluxToRadiance: 1}}}
	bands := map[string][]float64{"G": make([]float64, 3072)}

	csv := "hpx,n,ncolour,b_g\n0,1,1,\n1,1,1,-5\n2,1,1,7\n"

	if err := build.accumulate(mustRows(t, csv), bands, make([]int64, 3072), 1); err != nil {
		t.Fatalf("accumulate: %v", err)
	}

	if bands["G"][0] != 0 || bands["G"][1] != 0 {
		t.Errorf("unusable fluxes became %v and %v, want 0", bands["G"][0], bands["G"][1])
	}

	if bands["G"][2] != 7 {
		t.Errorf("the usable flux became %v, want 7", bands["G"][2])
	}
}

// mustRows builds a CSV row reader for a test body.
func mustRows(t *testing.T, body string) resultRows {
	t.Helper()

	rows, err := newCSVRows(strings.NewReader(body))
	if err != nil {
		t.Fatalf("newCSVRows: %v", err)
	}

	return rows
}
