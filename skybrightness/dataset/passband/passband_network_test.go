//go:build network

package passband_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness/dataset/passband"
)

// The five Johnson-Cousins bands as Table 1 of Masana, Bará, Carrasco & Ribas
// (2024), arXiv:2408.17371, characterises them: effective wavelength and
// effective width, in nanometres.
//
// This is what makes the comparison worth running. GAMBONS states which bands
// its published radiances are on but not which tabulation of them, and SVO
// carries several realisations of "Johnson-Cousins" that differ by a few
// nanometres. Checking the fetched curves against the paper's own numbers is
// what establishes that Generic/Bessell is the family the paper means, rather
// than assuming it because Bessell (1990) is the usual answer.
var table1 = []struct {
	filter   string
	centreNM float64
	widthNM  float64
}{
	{"Generic/Bessell.U", 361.6, 60.5},
	{"Generic/Bessell.B", 441.9, 95.9},
	{"Generic/Bessell.V", 552.4, 90.9},
	{"Generic/Bessell.R", 660.8, 163.4},
	{"Generic/Bessell.I", 802.0, 147.5},
}

// Tolerances on the comparison against Table 1, separate because the two
// statistics carry different weight.
//
// The centre is what identifies a band: neighbouring Johnson-Cousins filters
// are 90 nm apart at the blue end, so two per cent cannot confuse one for
// another. The width is looser because "effective width" has several
// definitions — SVO alone publishes WidthEff and FWHM, which differ by four
// nanometres for V — and the paper does not say which it used.
//
// # Why Generic/Bessell, measured rather than assumed
//
// "Johnson-Cousins" conventionally means Johnson UBV with Cousins RI, so the
// obvious reading is that R and I should come from Generic/Cousins. Measured
// against Table 1, they should not — the Cousins profiles are much further
// off than the Bessell ones:
//
//	band  Table 1 width   Bessell        Cousins/Johnson
//	U     60.5 nm         64.0 (+5.9%)   65.7 (+8.6%)   Johnson.U
//	B     95.9 nm         95.9 (+0.0%)   97.2 (+1.4%)   Johnson.B
//	V     90.9 nm         89.3 (-1.8%)   89.0 (-2.1%)   Johnson.V
//	R    163.4 nm        152.1 (-6.9%)  138.1 (-15.5%)  Cousins.R
//	I    147.5 nm        149.5 (+1.4%)  101.1 (-31.5%)  Cousins.I
//
// Bessell (1990) is closer in every band, and decisively so in I. That is the
// evidence for the identifiers below, and it is why the width tolerance is
// eight per cent rather than something tighter: U and R sit at six and seven,
// which is a difference of tabulation and not of band.
const (
	centreTolFrac = 0.02
	widthTolFrac  = 0.08
)

// Every Johnson-Cousins band GAMBONS tabulates resolves, and each is the band
// the paper says it is.
func TestFetchesTheJohnsonCousinsBands(t *testing.T) {
	testutil.RequireReachable(t, "svo2.cab.inta-csic.es:443")

	for _, want := range table1 {
		band, err := passband.Fetch(t.Context(), want.filter)
		if err != nil {
			t.Errorf("%s: %v", want.filter, err)

			continue
		}

		if err := band.Validate(); err != nil {
			t.Errorf("%s: does not validate: %v", want.filter, err)

			continue
		}

		centre, width := shape(band)

		t.Logf("%-20s centre %6.1f nm (Table 1: %6.1f), width %6.1f nm (Table 1: %6.1f), "+
			"detector %v, zero point %8.2f Jy",
			band.Name, centre, want.centreNM, width, want.widthNM, band.Detector,
			band.VegaZeroPointJy)

		if rel := math.Abs(centre-want.centreNM) / want.centreNM; rel > centreTolFrac {
			t.Errorf("%s: centre is %.1f nm against Table 1's %.1f, off by %.1f per cent — "+
				"this is not the band the paper tabulates",
				want.filter, centre, want.centreNM, 100*rel)
		}

		if rel := math.Abs(width-want.widthNM) / want.widthNM; rel > widthTolFrac {
			t.Errorf("%s: width is %.1f nm against Table 1's %.1f, off by %.1f per cent",
				want.filter, width, want.widthNM, 100*rel)
		}

		// The paper's magnitude scale for these five bands is Vega, which is
		// the system Passband.VegaZeroPointJy is on. A profile that came back
		// on AB would be a different calibration wearing the same name.
		if band.VegaZeroPointJy <= 0 {
			t.Errorf("%s: zero point is %g Jy", want.filter, band.VegaZeroPointJy)
		}
	}
}

// A second fetch is served from the cache, which is what keeps five bands at
// five requests rather than five per run.
func TestFetchIsCached(t *testing.T) {
	testutil.RequireReachable(t, "svo2.cab.inta-csic.es:443")

	const id = "Generic/Bessell.V"

	first, err := passband.Fetch(t.Context(), id)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	second, err := passband.Fetch(t.Context(), id)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if len(first.WavelengthNM) != len(second.WavelengthNM) {
		t.Fatalf("the two fetches returned %d and %d samples",
			len(first.WavelengthNM), len(second.WavelengthNM))
	}

	for i := range first.WavelengthNM {
		if first.WavelengthNM[i] != second.WavelengthNM[i] ||
			first.Response[i] != second.Response[i] {
			t.Fatalf("sample %d differs between the two fetches", i)
		}
	}

	if first.VegaZeroPointJy != second.VegaZeroPointJy || first.Detector != second.Detector {
		t.Error("the cached profile carries different metadata from the fetched one")
	}
}

// A filter nobody publishes is an error rather than an empty band.
func TestFetchRefusesAnUnknownFilter(t *testing.T) {
	testutil.RequireReachable(t, "svo2.cab.inta-csic.es:443")

	if band, err := passband.Fetch(t.Context(), "Generic/NoSuchFilter.X"); err == nil {
		t.Errorf("an unknown filter returned %q with %d samples",
			band.Name, len(band.WavelengthNM))
	}
}

// shape returns the response-weighted mean wavelength and the effective width,
// both computed from the curve rather than read from the metadata.
//
// Computing them here is the point: the metadata is what the service says, and
// this is what the samples actually are. A profile whose PARAM block and table
// disagreed would pass a metadata check and fail this one.
//
// The width is the standard one, the integral of the response over its peak,
// which for a tophat is exactly its span.
func shape(p magnitude.Passband) (centreNM, widthNM float64) {
	var num, den, peak float64

	for i := range p.WavelengthNM {
		var dl float64

		switch {
		case i == 0 && len(p.WavelengthNM) > 1:
			dl = float64(p.WavelengthNM[1] - p.WavelengthNM[0])
		case i == 0:
			dl = 1
		default:
			dl = float64(p.WavelengthNM[i] - p.WavelengthNM[i-1])
		}

		r := p.Response[i]
		num += float64(p.WavelengthNM[i]) * r * dl
		den += r * dl
		peak = math.Max(peak, r)
	}

	if den == 0 || peak == 0 {
		return 0, 0
	}

	return num / den, den / peak
}
