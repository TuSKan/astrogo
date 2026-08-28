package passband_test

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness/dataset/passband"
)

// profile renders a VOTable with the PARAM block where the service actually
// puts it: inside TABLE, not directly under RESOURCE.
//
// An earlier version of this helper put them under RESOURCE, which is where a
// reading of the VOTable schema suggests they belong. Every test here passed
// against it and every live fetch failed, because the parser had been written
// to the same wrong assumption as the fixture. A fixture is only worth having
// if it is the shape the service sends.
func profile(params map[string]string, rows [][2]float64) string {
	return voTable(params, rows, true)
}

// profileResourceLevel puts the PARAM block directly under RESOURCE, which
// VOTable also permits.
func profileResourceLevel(params map[string]string, rows [][2]float64) string {
	return voTable(params, rows, false)
}

func voTable(params map[string]string, rows [][2]float64, paramsInTable bool) string {
	var b strings.Builder

	writeParams := func() {
		for name, value := range params {
			b.WriteString(`<PARAM name="` + name + `" value="` + value + `"/>`)
		}
	}

	b.WriteString(`<?xml version="1.0"?><VOTABLE version="1.1"><RESOURCE type="results">`)

	if !paramsInTable {
		writeParams()
	}

	b.WriteString(`<TABLE utype="photdm:PhotometryFilter.transmissionCurve.spectrum">`)

	if paramsInTable {
		writeParams()
	}

	b.WriteString(`<FIELD name="Wavelength" unit="Angstrom" datatype="double"/>` +
		`<FIELD name="Transmission" datatype="double"/><DATA><TABLEDATA>`)

	for _, r := range rows {
		b.WriteString(`<TR><TD>` + ftoa(r[0]) + `</TD><TD>` + ftoa(r[1]) + `</TD></TR>`)
	}

	b.WriteString(`</TABLEDATA></DATA></TABLE></RESOURCE></VOTABLE>`)

	return b.String()
}

func ftoa(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// goodParams is a complete, valid metadata block: Bessell V as the service
// publishes it.
func goodParams() map[string]string {
	return map[string]string{
		"filterID":       "Generic/Bessell.V",
		"WavelengthUnit": "Angstrom",
		"DetectorType":   "0",
		"MagSys":         "Vega",
		"ZeroPoint":      "3630.2172842325",
		"ZeroPointUnit":  "Jy",
	}
}

// goodRows is a narrow but well-formed curve.
func goodRows() [][2]float64 {
	return [][2]float64{
		{4700, 0.0}, {5000, 0.458}, {5300, 1.0}, {5600, 0.79}, {6000, 0.20}, {6900, 0.0},
	}
}

// A profile parses into the passband the service describes, including the two
// things the curve alone cannot say.
func TestParseReadsTheProfile(t *testing.T) {
	t.Parallel()

	band, err := passband.Parse(strings.NewReader(profile(goodParams(), goodRows())))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if band.Name != "Generic/Bessell.V" {
		t.Errorf("name is %q, want the service's filter identifier", band.Name)
	}

	// Angstrom in, nanometres out. A profile read as nanometres would put V at
	// 5300 nm, which is thermal infrared rather than green.
	if len(band.WavelengthNM) != len(goodRows()) {
		t.Fatalf("%d samples, want %d", len(band.WavelengthNM), len(goodRows()))
	}

	if got := float64(band.WavelengthNM[2]); math.Abs(got-530) > 1e-9 {
		t.Errorf("peak sample is at %g nm, want 530 — the Angstrom conversion is wrong", got)
	}

	if band.Detector != magnitude.EnergyIntegrating {
		t.Errorf("detector is %v; the service said type 0, which is an energy counter",
			band.Detector)
	}

	if math.Abs(band.VegaZeroPointJy-3630.2172842325) > 1e-9 {
		t.Errorf("zero point is %g Jy, want the published 3630.2172842325",
			band.VegaZeroPointJy)
	}

	if err := band.Validate(); err != nil {
		t.Errorf("the parsed band does not validate: %v", err)
	}
}

// Metadata a passband cannot be built without is refused rather than defaulted.
//
// Each of these changes the answer in a way that cannot be undone downstream:
// the wrong wavelength unit is a factor of ten, the wrong detector convention
// tilts the result across the band, and the wrong magnitude system shifts every
// magnitude by a constant that looks entirely plausible.
func TestParseRefusesUnusableMetadata(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name    string
		breakIt func(map[string]string)
	}{
		{"wavelengths in nm", func(p map[string]string) { p["WavelengthUnit"] = "nm" }},
		{"no wavelength unit", func(p map[string]string) { delete(p, "WavelengthUnit") }},
		{"no detector type", func(p map[string]string) { delete(p, "DetectorType") }},
		{"unknown detector type", func(p map[string]string) { p["DetectorType"] = "2" }},
		{"AB zero point", func(p map[string]string) { p["MagSys"] = "AB" }},
		{"zero point in erg", func(p map[string]string) { p["ZeroPointUnit"] = "erg/cm2/s/A" }},
		{"no zero point", func(p map[string]string) { delete(p, "ZeroPoint") }},
		{"negative zero point", func(p map[string]string) { p["ZeroPoint"] = "-3630" }},
	} {
		p := goodParams()
		c.breakIt(p)

		if _, err := passband.Parse(strings.NewReader(profile(p, goodRows()))); err == nil {
			t.Errorf("%s: parsed without complaint", c.name)
		} else if !errors.Is(err, passband.ErrService) {
			t.Errorf("%s: err = %v, want ErrService", c.name, err)
		}
	}
}

// A body that is not a profile is an error, not an empty passband.
func TestParseRefusesRubbish(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ name, body string }{
		{"empty", ""},
		{"html error page", "<html><body>Service unavailable</body></html>"},
		{"truncated xml", `<VOTABLE><RESOURCE><PARAM name="filterID" value="x"/>`},
		{"no rows", profile(goodParams(), nil)},
	} {
		if _, err := passband.Parse(strings.NewReader(c.body)); err == nil {
			t.Errorf("%s: parsed without complaint", c.name)
		}
	}
}

// An identifier is required, since the service returns a page rather than an
// error when asked for nothing.
func TestFetchRefusesAnEmptyIdentifier(t *testing.T) {
	t.Parallel()

	if _, err := passband.Fetch(t.Context(), "   "); !errors.Is(err, passband.ErrService) {
		t.Errorf("err = %v, want ErrService", err)
	}
}

// Both placements of the PARAM block parse to the same passband.
//
// This is the regression test for the bug the fixture above describes: the
// parser must find the metadata wherever VOTable allows it to be, because a
// parser that looks in one place reports a missing unit rather than a
// structure it did not expect.
func TestParseFindsMetadataAtEitherDepth(t *testing.T) {
	t.Parallel()

	inTable, err := passband.Parse(strings.NewReader(profile(goodParams(), goodRows())))
	if err != nil {
		t.Fatalf("params inside TABLE: %v", err)
	}

	atResource, err := passband.Parse(
		strings.NewReader(profileResourceLevel(goodParams(), goodRows())))
	if err != nil {
		t.Fatalf("params under RESOURCE: %v", err)
	}

	if inTable.Name != atResource.Name ||
		inTable.Detector != atResource.Detector ||
		inTable.VegaZeroPointJy != atResource.VegaZeroPointJy ||
		len(inTable.WavelengthNM) != len(atResource.WavelengthNM) {
		t.Errorf("the two placements parsed differently: %+v against %+v",
			inTable, atResource)
	}
}
