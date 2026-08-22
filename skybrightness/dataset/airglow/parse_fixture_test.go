package airglow_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
)

// ── A minimal FITS binary table writer ───────────────────────────────────────
//
// Parse had no positive-path test, because fits.Write is unimplemented and
// there is no fixture file in the tree — everything that exercised it needed
// the network. Encoding a table is about forty lines, and having it means the
// parser's screening can be tested against the malformed responses a service
// actually returns rather than only against a string that is not FITS at all.

const fitsBlock = 2880

// card renders one 80-column FITS header card.
func card(keyword, value string) string {
	if keyword == "END" {
		return fmt.Sprintf("%-80s", "END")
	}

	return fmt.Sprintf("%-8s= %-70s", keyword, value)[:80]
}

// pad extends b to a whole number of 2880-byte blocks, with spaces for a
// header and zeros for data.
func pad(b []byte, with byte) []byte {
	if rem := len(b) % fitsBlock; rem != 0 {
		b = append(b, bytes.Repeat([]byte{with}, fitsBlock-rem)...)
	}

	return b
}

// bintableFITS builds a FITS file holding one binary table of float64 columns.
func bintableFITS(names []string, columns [][]float64) []byte {
	rows := 0
	if len(columns) > 0 {
		rows = len(columns[0])
	}

	var primary strings.Builder

	primary.WriteString(card("SIMPLE", "T"))
	primary.WriteString(card("BITPIX", "8"))
	primary.WriteString(card("NAXIS", "0"))
	primary.WriteString(card("EXTEND", "T"))
	primary.WriteString(card("END", ""))

	var ext strings.Builder

	ext.WriteString(card("XTENSION", "'BINTABLE'"))
	ext.WriteString(card("BITPIX", "8"))
	ext.WriteString(card("NAXIS", "2"))
	ext.WriteString(card("NAXIS1", strconv.Itoa(8*len(columns))))
	ext.WriteString(card("NAXIS2", strconv.Itoa(rows)))
	ext.WriteString(card("PCOUNT", "0"))
	ext.WriteString(card("GCOUNT", "1"))
	ext.WriteString(card("TFIELDS", strconv.Itoa(len(columns))))

	for i, name := range names {
		ext.WriteString(card(fmt.Sprintf("TTYPE%d", i+1), "'"+name+"'"))
		ext.WriteString(card(fmt.Sprintf("TFORM%d", i+1), "'1D'"))
	}

	ext.WriteString(card("END", ""))

	out := pad([]byte(primary.String()), ' ')
	out = append(out, pad([]byte(ext.String()), ' ')...)

	// Rows laid out consecutively, each column big-endian as FITS requires.
	data := make([]byte, 0, rows*8*len(columns))
	buf := make([]byte, 8)

	for r := range rows {
		for _, col := range columns {
			binary.BigEndian.PutUint64(buf, math.Float64bits(col[r]))
			data = append(data, buf...)
		}
	}

	return append(out, pad(data, 0)...)
}

// A well-formed response parses, and the conversion it performs is the one the
// hand-worked test already pins.
func TestParseReadsAWellFormedTable(t *testing.T) {
	t.Parallel()

	lam := []float64{500, 501, 502, 503}
	ael := []float64{1e3, 2e3, 0, 4e3}
	arc := []float64{5e2, 5e2, 5e2, 5e2}

	got, err := airglow.Parse(bytes.NewReader(bintableFITS(
		[]string{"lam", "flux_ael", "flux_arc"},
		[][]float64{lam, ael, arc},
	)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(got.LambdaNM) != len(lam) {
		t.Fatalf("parsed %d rows, want %d", len(got.LambdaNM), len(lam))
	}

	for i, nm := range lam {
		if got.LambdaNM[i] != nm {
			t.Errorf("row %d wavelength = %g, want %g", i, got.LambdaNM[i], nm)
		}

		if got.Radiance[i] < 0 || math.IsNaN(got.Radiance[i]) {
			t.Errorf("row %d radiance = %g", i, got.Radiance[i])
		}
	}

	// More photons is more radiance, and the row with no emission line still
	// carries the continuum.
	if !(got.Radiance[1] > got.Radiance[0] && got.Radiance[0] > got.Radiance[2]) {
		t.Errorf("radiance does not follow the flux: %v", got.Radiance)
	}

	if got.Radiance[2] <= 0 {
		t.Error("a row with no emission line lost its continuum as well")
	}
}

// A sample that is not a number must be dropped, not carried.
//
// A NaN here does not stay local. It becomes a NaN radiance, which sums into
// the scene total and comes back out as a NaN magnitude, by which point
// nothing identifies where it entered. solar.Parse screens its own spectrum
// the same way.
func TestParseDropsUnusableSamples(t *testing.T) {
	t.Parallel()

	lam := []float64{500, 501, 502, 503, 504, 505}
	ael := []float64{1e3, math.NaN(), 1e3, 1e3, math.Inf(1), 1e3}
	arc := []float64{5e2, 5e2, 5e2, 5e2, 5e2, 5e2}

	// Two unusable wavelengths as well: one absent and one impossible.
	lam[2] = 0
	lam[3] = math.NaN()

	got, err := airglow.Parse(bytes.NewReader(bintableFITS(
		[]string{"lam", "flux_ael", "flux_arc"},
		[][]float64{lam, ael, arc},
	)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Rows 1, 2, 3 and 4 are all unusable for one reason or another; 0 and 5
	// survive.
	if len(got.LambdaNM) != 2 {
		t.Fatalf("kept %d rows (%v), want the 2 usable ones", len(got.LambdaNM), got.LambdaNM)
	}

	for i, v := range got.Radiance {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			t.Errorf("row %d survived screening with radiance %v", i, v)
		}
	}

	// The two arrays must stay in step, or every wavelength is reported with
	// its neighbour's radiance.
	if len(got.LambdaNM) != len(got.Radiance) {
		t.Errorf("%d wavelengths against %d radiances", len(got.LambdaNM), len(got.Radiance))
	}
}

// A negative flux is a subtraction artefact in the model that produced it, not
// emission removed from the sky, so it clamps rather than darkening the total.
func TestParseClampsNegativeFlux(t *testing.T) {
	t.Parallel()

	got, err := airglow.Parse(bytes.NewReader(bintableFITS(
		[]string{"lam", "flux_ael", "flux_arc"},
		[][]float64{
			{500, 501, 502},
			{-5e3, 1e3, 0},
			{0, 0, 0},
		},
	)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	for i, v := range got.Radiance {
		if v < 0 {
			t.Errorf("row %d has radiance %g; a negative airglow is not emission removed from the sky", i, v)
		}
	}
}

// A table with fewer than two usable rows is not a spectrum: At interpolates
// between samples and cannot do it with one.
func TestParseRefusesTooFewRows(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name          string
		lam, ael, arc []float64
	}{
		{"one row", []float64{500}, []float64{1e3}, []float64{0}},
		{
			"every row unusable",
			[]float64{math.NaN(), -1, 0},
			[]float64{1e3, 1e3, 1e3},
			[]float64{0, 0, 0},
		},
		{
			"only one row survives screening",
			[]float64{500, 501},
			[]float64{1e3, math.NaN()},
			[]float64{0, 0},
		},
	} {
		_, err := airglow.Parse(bytes.NewReader(bintableFITS(
			[]string{"lam", "flux_ael", "flux_arc"},
			[][]float64{c.lam, c.ael, c.arc},
		)))
		if err == nil {
			t.Errorf("%s: Parse accepted a table that is not a spectrum", c.name)
		}
	}
}

// ESO documents these columns in upper case and serves them in lower. Both
// spellings must work, since asking for the documented one and getting nothing
// looks like a missing table rather than a case mismatch.
func TestParseAcceptsEitherColumnCase(t *testing.T) {
	t.Parallel()

	for _, names := range [][]string{
		{"lam", "flux_ael", "flux_arc"},
		{"LAM", "FLUX_AEL", "FLUX_ARC"},
	} {
		got, err := airglow.Parse(bytes.NewReader(bintableFITS(names, [][]float64{
			{500, 501, 502},
			{1e3, 1e3, 1e3},
			{5e2, 5e2, 5e2},
		})))
		if err != nil {
			t.Errorf("columns named %v: %v", names, err)

			continue
		}

		if len(got.LambdaNM) != 3 {
			t.Errorf("columns named %v: parsed %d rows, want 3", names, len(got.LambdaNM))
		}
	}
}

// The photon-to-radiance conversion, exercised through Parse itself.
//
// TestPhotonConversionMatchesAHandWorkedValue computes the same conversion
// from constants and checks its own arithmetic; it never calls this package,
// so a factor of a thousand or a missing photon energy in Parse would not have
// disturbed it. This drives the identical worked example through the parser.
//
// SkyCalc reports photons s^-1 m^-2 um^-1 arcsec^-2. The radiance is that
// divided by a thousand for micrometres to nanometres, divided by the solid
// angle of a square arcsecond, and multiplied by the energy of one photon at
// its own wavelength.
func TestParseConversionMatchesTheWorkedExample(t *testing.T) {
	t.Parallel()

	const (
		perMicronPerArcsec2 = 160.0
		lambdaNM            = 550.0

		arcsec2PerSter = 4.254517e10
		planck         = 6.62607015e-34
		lightSpeed     = 2.99792458e8
	)

	want := perMicronPerArcsec2 / 1000 * arcsec2PerSter * (planck * lightSpeed / (lambdaNM * 1e-9))

	// The emission lines carry it all in one case and the continuum in the
	// other, because Parse adds the two and a dropped column would otherwise
	// look like a correct answer halved.
	for _, c := range []struct {
		name     string
		ael, arc float64
	}{
		{"all in the emission lines", perMicronPerArcsec2, 0},
		{"all in the continuum", 0, perMicronPerArcsec2},
		{"split between them", perMicronPerArcsec2 / 2, perMicronPerArcsec2 / 2},
	} {
		got, err := airglow.Parse(bytes.NewReader(bintableFITS(
			[]string{"lam", "flux_ael", "flux_arc"},
			[][]float64{
				{lambdaNM - 1, lambdaNM, lambdaNM + 1},
				{c.ael, c.ael, c.ael},
				{c.arc, c.arc, c.arc},
			},
		)))
		if err != nil {
			t.Fatalf("%s: Parse: %v", c.name, err)
		}

		radiance := got.At(lambdaNM)

		if rel := math.Abs(radiance-want) / want; rel > 1e-6 {
			t.Errorf("%s: %g ph/s/m^2/um/arcsec^2 at %g nm parsed to %.6g W m^-2 sr^-1 nm^-1, "+
				"want %.6g (%.3g relative)", c.name, perMicronPerArcsec2, lambdaNM, radiance, want, rel)
		}
	}

	// And the scale is linear, so a spectrum twice as bright is twice the
	// radiance rather than twice some offset.
	half, err := airglow.Parse(bytes.NewReader(bintableFITS(
		[]string{"lam", "flux_ael", "flux_arc"},
		[][]float64{{549, 550, 551}, {80, 80, 80}, {0, 0, 0}},
	)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// The same 1e-6 as above: want is built from this test's own Planck and
	// speed-of-light literals while the parser uses the values in constants,
	// and the two differ in their last digits. That is a 1.4e-8 disagreement
	// about physical constants, not about the conversion.
	if ratio := want / half.At(lambdaNM); math.Abs(ratio-2) > 1e-6 {
		t.Errorf("halving the photon flux changed the radiance by a factor of %.9f, want 2", ratio)
	}
}
