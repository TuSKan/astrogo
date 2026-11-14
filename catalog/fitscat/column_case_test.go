package fitscat

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// catalogFile writes a FITS file holding one binary table whose columns are
// the given names, in order, each an 8-character string (NAME-like) or a
// float64, and returns its path. Two rows: Betelgeuse and Rigel.
func catalogFile(t *testing.T, names ...string) string {
	t.Helper()

	return catalogFileIn(t, nil, names...)
}

// catalogFileIn is catalogFile with a TUNITn for each column units names. RA
// and DEC are written in that unit: hours, radians, arcseconds or degrees.
func catalogFileIn(t *testing.T, units map[string]string, names ...string) string {
	t.Helper()

	in := func(column string, degrees float64) float64 {
		switch units[column] {
		case "h", "hourangle":
			return degrees / 15
		case "rad":
			return degrees * math.Pi / 180
		case "arcsec":
			return degrees * 3600
		default:
			return degrees
		}
	}

	primary := fitsCard("SIMPLE", "T") + fitsCard("BITPIX", "8") + fitsCard("NAXIS", "0") +
		fitsCard("EXTEND", "T") + "END"

	text := func(name string) bool {
		n := strings.ToUpper(name)
		return n == "ID" || n == "NAME"
	}

	// Every column is eight bytes wide, text or float64.
	width := 8 * len(names)

	var columns strings.Builder

	for i, name := range names {
		form := "'D       '"
		if text(name) {
			form = "'8A      '"
		}

		n := strconv.Itoa(i + 1)
		columns.WriteString(fitsCard("TTYPE"+n, "'"+name+"'"))
		columns.WriteString(fitsCard("TFORM"+n, form))

		if u, ok := units[name]; ok {
			columns.WriteString(fitsCard("TUNIT"+n, "'"+u+"'"))
		}
	}

	header := fitsCard("XTENSION", "'BINTABLE'") + fitsCard("BITPIX", "8") + fitsCard("NAXIS", "2") +
		fitsCard("NAXIS1", strconv.Itoa(width)) +
		fitsCard("NAXIS2", "2") + fitsCard("PCOUNT", "0") + fitsCard("GCOUNT", "1") +
		fitsCard("TFIELDS", strconv.Itoa(len(names))) +
		columns.String() +
		"END"

	var payload bytes.Buffer

	for _, row := range []struct {
		id, name string
		ra, dec  float64
	}{
		{"HIP27989", "Betelgse", 88.79, 7.41},
		{"HIP24436", "Rigel   ", 78.63, -8.20},
	} {
		for _, name := range names {
			switch strings.ToUpper(name) {
			case "ID":
				payload.WriteString(row.id)
			case "NAME":
				payload.WriteString(row.name)
			case "RA":
				_ = binary.Write(&payload, binary.BigEndian, in(name, row.ra))
			default:
				_ = binary.Write(&payload, binary.BigEndian, in(name, row.dec))
			}
		}
	}

	data := fitsBlock([]byte(primary))
	data = append(data, fitsBlock([]byte(header))...)
	data = append(data, fitsBlock(payload.Bytes())...)

	path := filepath.Join(t.TempDir(), "catalog.fits")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	return path
}

// TestColumnNamesInAnyCaseLoad: a table naming its columns in lower case, as
// the Gaia archive writes its column names, loads as the upper-case one does.
// fits matched column names exactly, against the FITS Standard's rule that
// they compare without regard to case, so New refused this table with
// ErrMissingColumns.
func TestColumnNamesInAnyCaseLoad(t *testing.T) {
	t.Parallel()

	for _, names := range [][]string{
		{"ID", "NAME", "RA", "DEC"},
		{"id", "name", "ra", "dec"},
		{"Id", "Name", "Ra", "Dec"},
	} {
		p, err := New(catalogFile(t, names...))
		if err != nil {
			t.Errorf("columns %v: New: %v", names, err)

			continue
		}

		got, err := p.Resolve(context.Background(), "HIP24436")
		if err != nil {
			t.Errorf("columns %v: Resolve: %v", names, err)

			continue
		}

		if !got.HasCoord || got.Coord.RA().Degrees() != 78.63 || got.Name != "Rigel" {
			t.Errorf("columns %v: resolved %+v, want Rigel at RA 78.63 with a position", names, got)
		}
	}
}

// TestMissingColumnIsNamed: a table without one of the four columns is refused,
// and the error says which. It used to be a bare ErrMissingColumns, leaving a
// caller to guess which column and whether it was absent or the wrong type.
func TestMissingColumnIsNamed(t *testing.T) {
	t.Parallel()

	_, err := New(catalogFile(t, "ID", "NAME", "RA"))
	if !errors.Is(err, ErrMissingColumns) {
		t.Fatalf("New without DEC: err = %v, want ErrMissingColumns", err)
	}

	if !errors.Is(err, fits.ErrColumnNotFound) || !strings.Contains(err.Error(), "DEC") {
		t.Errorf("New without DEC: err = %v, want it to name the DEC column as not found", err)
	}
}

// TestCoordinateColumnUnits: RA and DEC are read in the unit their TUNITn
// declares. Every column was read as degrees, so a right ascension stored in
// hours put Rigel's 5.24 h at 5.24 degrees, 73 degrees of right ascension
// from Rigel, and no range check can see it: hours and radians both lie
// inside a full turn.
func TestCoordinateColumnUnits(t *testing.T) {
	t.Parallel()

	names := []string{"ID", "NAME", "RA", "DEC"}

	for _, units := range []map[string]string{
		nil,
		{"RA": "deg", "DEC": "deg"},
		{"RA": "h", "DEC": "deg"},
		{"RA": "hourangle", "DEC": "deg"},
		{"RA": "rad", "DEC": "rad"},
		{"RA": "arcsec", "DEC": "arcsec"},
	} {
		p, err := New(catalogFileIn(t, units, names...))
		if err != nil {
			t.Errorf("units %v: New: %v", units, err)

			continue
		}

		got, err := p.Resolve(context.Background(), "Rigel")
		if err != nil {
			t.Errorf("units %v: Resolve: %v", units, err)

			continue
		}

		ra, dec := got.Coord.RA().Degrees(), got.Coord.Dec().Degrees()
		if !got.HasCoord || math.Abs(ra-78.63) > 1e-9 || math.Abs(dec-(-8.20)) > 1e-9 {
			t.Errorf("units %v: Rigel at RA %.9g, Dec %.9g (HasCoord %v), want 78.63, -8.2",
				units, ra, dec, got.HasCoord)
		}
	}

	// A unit that is not an angle, and a declination in hours, are refused:
	// there is no right reading of either.
	for _, units := range []map[string]string{
		{"RA": "m", "DEC": "deg"},
		{"RA": "deg", "DEC": "h"},
	} {
		if _, err := New(catalogFileIn(t, units, names...)); !errors.Is(err, ErrColumnUnit) {
			t.Errorf("units %v: err = %v, want ErrColumnUnit", units, err)
		}
	}
}
