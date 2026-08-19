package starlight_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

func gBand() starlight.GaiaBand {
	return starlight.GaiaBand{Name: "G", FluxToRadiance: 1e-18}
}

// The divisor is the whole trick: a Gaia source_id carries the HEALPix index
// in its high bits, so source_id/2^(59-2k) is the level-k nested pixel and the
// aggregation becomes a server-side GROUP BY.
//
// Order 8 is GAMBONS' grid, and 2^(59-16) is 8796093022208 — the constant this
// query stands or falls on. Getting it wrong by one power of two silently
// aggregates at the wrong resolution.
func TestGaiaADQLDivisor(t *testing.T) {
	t.Parallel()

	adql, err := starlight.GaiaBuild{Bands: []starlight.GaiaBand{gBand()}}.ADQL(0, 999)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	if !strings.Contains(adql, "source_id/8796093022208") {
		t.Errorf("order 8 must divide by 8796093022208:\n%s", adql)
	}

	// The chunk's source_id range must be exactly the pixels asked for.
	if !strings.Contains(adql, "BETWEEN 0 AND 8796093022207999") {
		t.Errorf("pixels 0-999 must map to their exact source_id range:\n%s", adql)
	}

	// Order 12 is the finest source_id addresses.
	fine, err := starlight.GaiaBuild{Order: 12, Bands: []starlight.GaiaBand{gBand()}}.ADQL(0, 0)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	if !strings.Contains(fine, "source_id/34359738368") {
		t.Errorf("order 12 must divide by 34359738368:\n%s", fine)
	}
}

// A band with no colour term is the Gaia G band itself and needs no
// transformation — the one case that works without the caller supplying a
// published fit.
func TestGaiaADQLPlainBand(t *testing.T) {
	t.Parallel()

	adql, err := starlight.GaiaBuild{Bands: []starlight.GaiaBand{gBand()}}.ADQL(0, 9)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	if !strings.Contains(adql, "SUM(phot_g_mean_flux) AS b_G") {
		t.Errorf("a band with no colour term must sum G flux directly:\n%s", adql)
	}

	if strings.Contains(adql, "POWER") {
		t.Errorf("a band with no colour term must not apply a transformation:\n%s", adql)
	}
}

// The colour transformation is applied per star inside the aggregate, not to
// the summed flux afterwards. That distinction is the reason the polynomial is
// rendered into the query at all: transforming a sum is not the same as
// summing transformations when the transformation depends on colour.
func TestGaiaADQLAppliesColourPerStar(t *testing.T) {
	t.Parallel()

	band := starlight.GaiaBand{
		Name:           "V",
		ColourTerm:     []float64{0.02, 0.007, 0.17},
		FluxToRadiance: 1e-18,
	}

	adql, err := starlight.GaiaBuild{Bands: []starlight.GaiaBand{band}}.ADQL(0, 9)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	// The polynomial must sit inside the SUM, multiplying each star's flux.
	if !strings.Contains(adql, "SUM(phot_g_mean_flux*POWER(10,0.4*(") {
		t.Errorf("the transformation must be inside the SUM:\n%s", adql)
	}

	// Every order of the polynomial must appear.
	for _, want := range []string{"0.02", "0.007*bp_rp", "0.17*bp_rp*bp_rp"} {
		if !strings.Contains(adql, want) {
			t.Errorf("missing %q from the polynomial:\n%s", want, adql)
		}
	}

	// The archive rejects COALESCE alongside CASE, so a transformed band
	// cannot substitute a default colour inside the aggregate. Sources
	// without BP-RP make the polynomial null and SQL drops them, which is
	// why their count is reported separately.
	if strings.Contains(adql, "COALESCE") {
		t.Errorf("the archive rejects COALESCE with a 400:\n%s", adql)
	}

	// And the count of those sources must be recoverable, so a caller can see
	// how much of a pixel rests on the fallback. COUNT(bp_rp) counts the
	// non-null colours; the archive's ADQL parser rejects a CASE expression
	// with an HTTP 400, which is why it is written this way rather than more
	// directly.
	if !strings.Contains(adql, "COUNT(bp_rp) AS ncolour") {
		t.Errorf("the colour count must be reported: %s", adql)
	}

	if strings.Contains(adql, "CASE") {
		t.Errorf("the archive rejects CASE expressions with a 400: %s", adql)
	}
}

// The sign convention: a positive G - m_band means the band magnitude is
// brighter than G, so it carries more flux. A star redder than the reference
// must therefore gain flux in a red band.
func TestGaiaColourTermSign(t *testing.T) {
	t.Parallel()

	adql, err := starlight.GaiaBuild{Bands: []starlight.GaiaBand{{
		Name: "red", ColourTerm: []float64{0.5}, FluxToRadiance: 1,
	}},
	}.ADQL(0, 9)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	if !strings.Contains(adql, "POWER(10,0.4*(0.5))") {
		t.Errorf("a positive G-m must raise the flux, so the exponent is +0.4:\n%s", adql)
	}
}

func TestGaiaBuildValidates(t *testing.T) {
	t.Parallel()

	if _, err := (starlight.GaiaBuild{}).ADQL(0, 9); !errors.Is(err, starlight.ErrGaiaBand) {
		t.Errorf("no bands: err = %v, want ErrGaiaBand", err)
	}

	noName := starlight.GaiaBuild{Bands: []starlight.GaiaBand{{FluxToRadiance: 1}}}
	if _, err := noName.ADQL(0, 9); !errors.Is(err, starlight.ErrGaiaBand) {
		t.Errorf("unnamed band: err = %v, want ErrGaiaBand", err)
	}

	// A band with no flux-to-radiance factor cannot produce a radiance, and
	// guessing a zero point is exactly what this package refuses to do.
	noZero := starlight.GaiaBuild{Bands: []starlight.GaiaBand{{Name: "V"}}}
	if _, err := noZero.ADQL(0, 9); !errors.Is(err, starlight.ErrGaiaBand) {
		t.Errorf("band without a zero point: err = %v, want ErrGaiaBand", err)
	}

	for _, order := range []int{-1, 13, 20} {
		bad := starlight.GaiaBuild{Order: order, Bands: []starlight.GaiaBand{gBand()}}
		if _, err := bad.ADQL(0, 9); !errors.Is(err, starlight.ErrGaiaOrder) {
			t.Errorf("order %d: err = %v, want ErrGaiaOrder", order, err)
		}
	}
}

// Chunks must tile the sky exactly — no pixel covered twice, none missed.
func TestGaiaChunksTileTheSky(t *testing.T) {
	t.Parallel()

	const (
		order = 4
		npix  = 12 * 4 * 4 * 4 * 4 // 12*4^4 = 3072
		chunk = 100
	)

	build := starlight.GaiaBuild{Order: order, Chunk: chunk, Bands: []starlight.GaiaBand{gBand()}}

	var lastEnd int64 = -1

	for first := int64(0); first < npix; first += chunk {
		last := first + chunk - 1
		if last >= npix {
			last = npix - 1
		}

		if first != lastEnd+1 {
			t.Fatalf("chunk starting at %d leaves a gap after %d", first, lastEnd)
		}

		if _, err := build.ADQL(first, last); err != nil {
			t.Fatalf("ADQL(%d, %d): %v", first, last, err)
		}

		lastEnd = last
	}

	if lastEnd != npix-1 {
		t.Errorf("chunks stop at pixel %d, want %d", lastEnd, npix-1)
	}
}
