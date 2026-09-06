package solar_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
)

// The parse path had 14% coverage while being the only part of this package
// that touches a file somebody else produced — and CLAUDE.md puts the dataset
// tier's I/O exactly where coverage should be highest, because parse errors,
// truncation and schema drift are what actually happen in the field (#122).
//
// Everything here is a hand-built FITS byte slice, so it runs offline and
// deterministically. fits.Write returns ErrUnimplemented (#127), so a fixture
// has to be assembled rather than round-tripped; the shape is copied from
// fits/bintable_decode_test.go's own synthetic builder, which cannot be
// imported across the package boundary.

// card renders one 80-byte FITS header card.
func card(keyword, value string) string {
	line := keyword
	for len(line) < 8 {
		line += " "
	}

	line += "= " + value

	for len(line) < 80 {
		line += " "
	}

	return line[:80]
}

// pad rounds a block out to the 2880-byte FITS block size.
func pad(b []byte) []byte {
	if r := len(b) % 2880; r != 0 {
		b = append(b, bytes.Repeat([]byte{' '}, 2880-r)...)
	}

	return b
}

// calspecLike builds a FITS file shaped like a CALSPEC solar reference: an
// empty primary HDU and a BINTABLE carrying WAVELENGTH (float64, angstrom) and
// FLUX (float64, erg s^-1 cm^-2 angstrom^-1).
//
// The column names are padded the way FITS pads a string to its TFORM width,
// because that padding is the thing floatColumn's case-folded trim exists for
// and an exact-match lookup would find neither column.
func calspecLike(t *testing.T, wavelengthA, fluxErg []float64) []byte {
	t.Helper()

	if len(wavelengthA) != len(fluxErg) {
		t.Fatalf("fixture: %d wavelengths and %d fluxes", len(wavelengthA), len(fluxErg))
	}

	primary := card("SIMPLE", "T") + card("BITPIX", "8") + card("NAXIS", "0") +
		card("EXTEND", "T") + "END"

	const rowSize = 8 + 8 // two D columns

	header := card("XTENSION", "'BINTABLE'") +
		card("BITPIX", "8") +
		card("NAXIS", "2") +
		card("NAXIS1", "16") +
		card("NAXIS2", itoa(len(wavelengthA))) +
		card("PCOUNT", "0") +
		card("GCOUNT", "1") +
		card("TFIELDS", "2") +
		card("TTYPE1", "'WAVELENGTH '") + card("TFORM1", "'D       '") +
		card("TTYPE2", "'FLUX    '") + card("TFORM2", "'D       '") +
		"END"

	if rowSize != 16 {
		t.Fatalf("fixture row is %d bytes, header declares 16", rowSize)
	}

	var payload bytes.Buffer

	for i := range wavelengthA {
		_ = binary.Write(&payload, binary.BigEndian, wavelengthA[i])
		_ = binary.Write(&payload, binary.BigEndian, fluxErg[i])
	}

	out := pad([]byte(primary))
	out = append(out, pad([]byte(header))...)

	return append(out, pad(payload.Bytes())...)
}

// itoa avoids importing strconv for four call sites of small positive numbers.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}

// TestParseConvertsCALSPECUnits is the assertion that matters most, because a
// wrong factor here is a plausible-looking spectrum rather than an error.
//
// CALSPEC publishes wavelength in angstrom and flux in
// erg s^-1 cm^-2 angstrom^-1. Derived independently of the code: one erg is
// 1e-7 J, one cm^-2 is 1e4 m^-2, and one angstrom^-1 is 10 nm^-1, so the flux
// factor is 1e-7 * 1e4 * 10 = 1e-2 and the wavelength factor is 0.1.
func TestParseConvertsCALSPECUnits(t *testing.T) {
	t.Parallel()

	// 4000 Å is 400 nm; 2.5 erg... is 0.025 W m^-2 nm^-1.
	raw := calspecLike(t,
		[]float64{4000, 5000, 6000},
		[]float64{2.5, 4.0, 1.0},
	)

	spec, err := solar.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := len(spec.WavelengthNM); got != 3 {
		t.Fatalf("kept %d rows, want 3", got)
	}

	for i, want := range []float64{400, 500, 600} {
		if got := float64(spec.WavelengthNM[i]); math.Abs(got-want) > 1e-9 {
			t.Errorf("wavelength[%d] = %g nm, want %g — the angstrom factor is wrong", i, got, want)
		}
	}

	for i, want := range []float64{0.025, 0.040, 0.010} {
		if got := spec.Irradiance[i]; math.Abs(got-want) > 1e-12 {
			t.Errorf("irradiance[%d] = %g W m^-2 nm^-1, want %g — the erg factor is wrong", i, got, want)
		}
	}
}

// TestParseDropsRowsItCannotUse covers the three filters, each of which is a
// real property of published spectra rather than defensive programming.
func TestParseDropsRowsItCannotUse(t *testing.T) {
	t.Parallel()

	raw := calspecLike(t,
		[]float64{4000, math.NaN(), 0, -100, 5000, 6000},
		[]float64{2.5, 1.0, 1.0, 1.0, math.NaN(), 3.0},
	)

	spec, err := solar.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Kept: 4000 and 6000. Dropped: NaN wavelength, zero, negative, and the
	// row whose flux is NaN.
	if got := len(spec.WavelengthNM); got != 2 {
		t.Fatalf("kept %d rows, want 2 (4000 Å and 6000 Å)", got)
	}

	if got := float64(spec.WavelengthNM[0]); math.Abs(got-400) > 1e-9 {
		t.Errorf("first kept wavelength = %g nm, want 400", got)
	}

	if got := float64(spec.WavelengthNM[1]); math.Abs(got-600) > 1e-9 {
		t.Errorf("second kept wavelength = %g nm, want 600 — a dropped row shifted the pairing", got)
	}
}

// TestParseClampsNegativeFlux pins the documented decision that a negative
// flux is a calibration artefact rather than a measurement.
//
// Clamping to zero and dropping the row are different answers: dropping would
// leave a gap in the wavelength grid that At would then interpolate across.
func TestParseClampsNegativeFlux(t *testing.T) {
	t.Parallel()

	raw := calspecLike(t,
		[]float64{4000, 5000, 6000},
		[]float64{2.5, -3.0, 1.0},
	)

	spec, err := solar.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := len(spec.WavelengthNM); got != 3 {
		t.Fatalf("kept %d rows, want 3 — a negative flux is clamped, not dropped", got)
	}

	if got := spec.Irradiance[1]; got != 0 {
		t.Errorf("negative flux became %g, want 0", got)
	}
}

// TestParseRefusesAFileWithNoUsableSpectrum covers the two ways a file can
// parse as FITS and still carry nothing to interpolate.
func TestParseRefusesAFileWithNoUsableSpectrum(t *testing.T) {
	t.Parallel()

	t.Run("fewer than two usable rows", func(t *testing.T) {
		t.Parallel()

		raw := calspecLike(t, []float64{4000, -1}, []float64{2.5, 1.0})

		if _, err := solar.Parse(bytes.NewReader(raw)); !errors.Is(err, solar.ErrNoSpectrum) {
			t.Errorf("Parse returned %v, want ErrNoSpectrum — one point cannot be interpolated", err)
		}
	})

	t.Run("no binary table at all", func(t *testing.T) {
		t.Parallel()

		primary := card("SIMPLE", "T") + card("BITPIX", "8") + card("NAXIS", "0") + "END"

		if _, err := solar.Parse(bytes.NewReader(pad([]byte(primary)))); !errors.Is(err, solar.ErrNoSpectrum) {
			t.Errorf("Parse returned %v, want ErrNoSpectrum", err)
		}
	})

	t.Run("not FITS", func(t *testing.T) {
		t.Parallel()

		if _, err := solar.Parse(bytes.NewReader([]byte("<html>503</html>"))); err == nil {
			t.Error("Parse accepted a document that is not FITS")
		}
	})
}

// TestParseMatchesPaddedColumnNames is the one that would have been missed by
// reading the code rather than a file.
//
// FITS pads a string value to the width its TFORM declares, so CALSPEC's
// columns arrive as "WAVELENGTH " and "FLUX    ". An exact-match lookup finds
// neither, and the file then looks like one carrying no spectrum at all —
// ErrNoSpectrum for a perfectly good file. The fixture above is padded for
// exactly this reason; here the names are also cased differently, since the
// lookup folds case as well as trimming.
func TestParseMatchesPaddedColumnNames(t *testing.T) {
	t.Parallel()

	primary := card("SIMPLE", "T") + card("BITPIX", "8") + card("NAXIS", "0") +
		card("EXTEND", "T") + "END"

	header := card("XTENSION", "'BINTABLE'") +
		card("BITPIX", "8") +
		card("NAXIS", "2") +
		card("NAXIS1", "16") +
		card("NAXIS2", "2") +
		card("PCOUNT", "0") +
		card("GCOUNT", "1") +
		card("TFIELDS", "2") +
		card("TTYPE1", "'wavelength   '") + card("TFORM1", "'D       '") +
		card("TTYPE2", "'  Flux      '") + card("TFORM2", "'D       '") +
		"END"

	var payload bytes.Buffer

	for i, w := range []float64{4000.0, 5000.0} {
		_ = binary.Write(&payload, binary.BigEndian, w)
		_ = binary.Write(&payload, binary.BigEndian, 1.0+float64(i))
	}

	raw := pad([]byte(primary))
	raw = append(raw, pad([]byte(header))...)
	raw = append(raw, pad(payload.Bytes())...)

	spec, err := solar.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse rejected lower-cased, space-padded column names: %v\n"+
			"  FITS pads a string to its TFORM width, so a real CALSPEC file's columns "+
			"arrive padded; an exact-match lookup reports a good file as empty.", err)
	}

	if got := len(spec.WavelengthNM); got != 2 {
		t.Errorf("kept %d rows, want 2", got)
	}
}

// TestParseReadsFluxAtEitherWidth covers floatColumn's documented promise to
// read a column "whatever width it was stored at".
//
// CALSPEC products are not uniform on this: some carry FLUX as float32 (TFORM
// 'E') and some as float64 ('D'). A reader that handled only one would work
// against whichever file it was developed on and report an empty spectrum for
// the other — ErrNoSpectrum for a good file, which is the same silent failure
// the padded-column-name case produces.
func TestParseReadsFluxAtEitherWidth(t *testing.T) {
	t.Parallel()

	primary := card("SIMPLE", "T") + card("BITPIX", "8") + card("NAXIS", "0") +
		card("EXTEND", "T") + "END"

	// WAVELENGTH as float64, FLUX as float32: 8 + 4 = 12 bytes per row.
	header := card("XTENSION", "'BINTABLE'") +
		card("BITPIX", "8") +
		card("NAXIS", "2") +
		card("NAXIS1", "12") +
		card("NAXIS2", "2") +
		card("PCOUNT", "0") +
		card("GCOUNT", "1") +
		card("TFIELDS", "2") +
		card("TTYPE1", "'WAVELENGTH '") + card("TFORM1", "'D       '") +
		card("TTYPE2", "'FLUX    '") + card("TFORM2", "'E       '") +
		"END"

	var payload bytes.Buffer

	for i, w := range []float64{4000.0, 5000.0} {
		_ = binary.Write(&payload, binary.BigEndian, w)
		_ = binary.Write(&payload, binary.BigEndian, float32(2.5+float64(i)))
	}

	raw := pad([]byte(primary))
	raw = append(raw, pad([]byte(header))...)
	raw = append(raw, pad(payload.Bytes())...)

	spec, err := solar.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse rejected a float32 FLUX column: %v", err)
	}

	if got := len(spec.Irradiance); got != 2 {
		t.Fatalf("kept %d rows, want 2", got)
	}

	// 2.5 erg... is 0.025 W m^-2 nm^-1, the same conversion as the float64
	// case, so a width-dependent factor would show up here.
	if got := spec.Irradiance[0]; math.Abs(got-0.025) > 1e-9 {
		t.Errorf("irradiance[0] = %g, want 0.025", got)
	}
}

// TestParseIgnoresANonNumericFluxColumn covers floatColumn's default branch.
//
// A column of the right name and the wrong type is schema drift, and it must
// read as "this file has no spectrum" rather than as zeros — which would be a
// perfectly plausible dark spectrum.
func TestParseIgnoresANonNumericFluxColumn(t *testing.T) {
	t.Parallel()

	primary := card("SIMPLE", "T") + card("BITPIX", "8") + card("NAXIS", "0") +
		card("EXTEND", "T") + "END"

	// FLUX as a 6-character string column: named right, unusable.
	header := card("XTENSION", "'BINTABLE'") +
		card("BITPIX", "8") +
		card("NAXIS", "2") +
		card("NAXIS1", "14") +
		card("NAXIS2", "2") +
		card("PCOUNT", "0") +
		card("GCOUNT", "1") +
		card("TFIELDS", "2") +
		card("TTYPE1", "'WAVELENGTH '") + card("TFORM1", "'D       '") +
		card("TTYPE2", "'FLUX    '") + card("TFORM2", "'6A      '") +
		"END"

	var payload bytes.Buffer

	for _, w := range []float64{4000.0, 5000.0} {
		_ = binary.Write(&payload, binary.BigEndian, w)
		payload.WriteString("bright")
	}

	raw := pad([]byte(primary))
	raw = append(raw, pad([]byte(header))...)
	raw = append(raw, pad(payload.Bytes())...)

	if _, err := solar.Parse(bytes.NewReader(raw)); !errors.Is(err, solar.ErrNoSpectrum) {
		t.Errorf("Parse returned %v for a FLUX column of the wrong type, want ErrNoSpectrum.\n"+
			"  Reading it as zeros would be a plausible dark spectrum rather than an error.", err)
	}
}
