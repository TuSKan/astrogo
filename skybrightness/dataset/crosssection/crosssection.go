// Package crosssection reads molecular absorption cross sections into
// [atmosphere.CrossSection].
//
// # Why no dataset is shipped or defaulted
//
// The MPI-Mainz UV/VIS Spectral Atlas holds dozens of measurements for each
// of O3, O2 and H2O, by different authors, at different temperatures and over
// different wavelength ranges. Ozone's cross section is strongly
// temperature-dependent through the Chappuis band that matters at optical
// wavelengths, so "the ozone cross section" is not a thing — a reference and
// a temperature are a scientific choice, and one this package makes the
// caller state rather than making silently.
//
// astrogo therefore ships the reader and the caller supplies the file, the
// same arrangement as the solar spectrum, the airglow spectrum and the Gaia
// band transformations.
//
// The atlas is at https://www.uv-vis-spectral-atlas-mainz.org and should be
// cited as Keller-Rudek, H., Moortgat, G.K., Sander, R. and Sörensen, R.,
// Earth Syst. Sci. Data 5, 365 (2013).
package crosssection

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for cross-section parsing.
var (
	// ErrFormat is returned when a file holds no usable wavelength and
	// cross-section pairs.
	ErrFormat = errors.New("crosssection: file has no usable data rows")

	// ErrUnit is returned for an unrecognised wavelength unit.
	ErrUnit = errors.New("crosssection: unrecognised wavelength unit")
)

// WavelengthUnit names what the first column of a file holds.
//
// The atlas is not uniform about this: some files are in nanometres, others
// in angstrom, others in wavenumbers. Nothing in the file body says which,
// so it is a parameter rather than something to sniff — guessing wrong
// shifts every absorption feature by a factor of ten and still produces a
// plausible-looking curve.
type WavelengthUnit string

// The wavelength units the atlas uses.
const (
	Nanometre  WavelengthUnit = "nm"
	Angstrom   WavelengthUnit = "A"
	Wavenumber WavelengthUnit = "cm-1"
)

// toNM converts one value in the given unit to nanometres.
func (u WavelengthUnit) toNM(v float64) (float64, error) {
	switch u {
	case Nanometre:
		return v, nil
	case Angstrom:
		return v / 10, nil
	case Wavenumber:
		if v <= 0 {
			return 0, fmt.Errorf("%w: wavenumber %g", ErrFormat, v)
		}

		// 1 cm^-1 corresponds to 1e7 nm.
		return 1e7 / v, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrUnit, u)
	}
}

// Parse reads a two-column ASCII cross-section table into an
// [atmosphere.CrossSection].
//
// The format is what the MPI-Mainz atlas publishes: a wavelength and a cross
// section per line, whitespace- or comma-separated, with comment and header
// lines that do not parse as two numbers. Those are skipped rather than
// rejected, because the atlas's files carry provenance headers in no fixed
// form and refusing them would mean hand-editing every download.
//
// Cross sections are in cm^2 per molecule, which is what the atlas uses and
// what [atmosphere.CrossSection] expects.
//
// Rows are sorted by wavelength and duplicates dropped, since a file given in
// wavenumbers arrives in descending wavelength order and
// [atmosphere.CrossSection] requires strictly increasing.
func Parse(r io.Reader, species string, u WavelengthUnit) (atmosphere.CrossSection, error) {
	if _, err := u.toNM(1); err != nil {
		return atmosphere.CrossSection{}, err
	}

	type row struct{ nm, sigma float64 }

	var rows []row

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		nm, sigma, ok := parseRow(scanner.Text(), u)
		if !ok {
			continue
		}

		rows = append(rows, row{nm, sigma})
	}

	if err := scanner.Err(); err != nil {
		return atmosphere.CrossSection{}, fmt.Errorf("crosssection: read: %w", err)
	}

	if len(rows) < 2 {
		return atmosphere.CrossSection{}, fmt.Errorf("%w: %d rows", ErrFormat, len(rows))
	}

	// Ascending wavelength, duplicates dropped. A file given in wavenumbers
	// arrives in descending wavelength order, and CrossSection requires
	// strictly increasing.
	slices.SortFunc(rows, func(a, b row) int { return cmp.Compare(a.nm, b.nm) })

	out := atmosphere.CrossSection{
		Species:      species,
		WavelengthNM: make([]unit.WavelengthNM, 0, len(rows)),
		SigmaCM2:     make([]float64, 0, len(rows)),
	}

	for i, r := range rows {
		if i > 0 && r.nm <= rows[i-1].nm {
			continue
		}

		out.WavelengthNM = append(out.WavelengthNM, unit.WavelengthNM(r.nm))
		out.SigmaCM2 = append(out.SigmaCM2, r.sigma)
	}

	if err := out.Validate(); err != nil {
		return atmosphere.CrossSection{}, fmt.Errorf("crosssection: %w", err)
	}

	return out, nil
}

// parseRow reads one line, reporting whether it held a usable pair.
func parseRow(line string, u WavelengthUnit) (nm, sigma float64, ok bool) {
	fields := strings.FieldsFunc(strings.TrimSpace(line), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == ';'
	})

	if len(fields) < 2 {
		return 0, 0, false
	}

	first, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, false
	}

	second, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, false
	}

	nm, err = u.toNM(first)
	if err != nil {
		return 0, 0, false
	}

	// A negative cross section is a measurement artefact at the noise floor,
	// not absorption; clamping keeps the optical depth physical.
	if second < 0 {
		second = 0
	}

	if nm <= 0 || math.IsNaN(nm) || math.IsNaN(second) || math.IsInf(second, 0) {
		return 0, 0, false
	}

	return nm, second, true
}
