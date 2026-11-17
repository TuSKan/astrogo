package fitscat

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
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
				_ = binary.Write(&payload, binary.BigEndian, row.ra)
			default:
				_ = binary.Write(&payload, binary.BigEndian, row.dec)
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
