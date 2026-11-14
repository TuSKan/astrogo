package fits_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strconv"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// caseTable is a BINTABLE of three float64 columns, "ra", "flux" and "FLUX",
// one row: 10, 20, 30. The last two differ only in case, which the FITS
// Standard strongly recommends against and does not forbid.
func caseTable(t *testing.T) *fits.BintableHDU {
	t.Helper()

	primary := card("SIMPLE", "T") + card("BITPIX", "8") + card("NAXIS", "0") +
		card("EXTEND", "T") + "END"

	header := card("XTENSION", "'BINTABLE'") +
		card("BITPIX", "8") +
		card("NAXIS", "2") +
		card("NAXIS1", "24") +
		card("NAXIS2", "1") +
		card("PCOUNT", "0") +
		card("GCOUNT", "1") +
		card("TFIELDS", "3") +
		card("TTYPE1", "'ra      '") + card("TFORM1", "'D       '") +
		card("TTYPE2", "'flux    '") + card("TFORM2", "'D       '") +
		card("TTYPE3", "'FLUX    '") + card("TFORM3", "'D       '") +
		"END"

	var payload bytes.Buffer

	_ = binary.Write(&payload, binary.BigEndian, [3]float64{10, 20, 30})

	data := pad([]byte(primary))
	data = append(data, pad([]byte(header))...)
	data = append(data, pad(payload.Bytes())...)

	f, err := fits.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	table, ok := f.HDUs[1].(*fits.BintableHDU)
	if !ok {
		t.Fatalf("HDU 1 is %T, want *fits.BintableHDU", f.HDUs[1])
	}

	return table
}

// TestColumnNamesMatchWithoutCase: the FITS Standard (4.0, §7.3.2) says
// comparisons with TTYPEn values should not be case sensitive, and both
// getters matched exactly, so a table naming its column "ra" had no "RA". An
// exact match still wins, so a table holding "flux" and "FLUX" returns the one
// asked for.
func TestColumnNamesMatchWithoutCase(t *testing.T) {
	t.Parallel()

	table := caseTable(t)

	for _, c := range []struct {
		name string
		want float64
	}{
		{"ra", 10},
		{"RA", 10},
		{"Ra", 10},
		{"flux", 20},
		{"FLUX", 30},
		// No exact match: the first column matching without case.
		{"Flux", 20},
	} {
		got, err := table.GetFloatColumn(c.name)
		if err != nil {
			t.Errorf("GetFloatColumn(%q): %v", c.name, err)

			continue
		}

		if len(got) != 1 || got[0] != c.want {
			t.Errorf("GetFloatColumn(%q) = %v, want [%g]", c.name, got, c.want)
		}

		text, err := table.GetStringColumn(c.name)
		if err != nil {
			t.Errorf("GetStringColumn(%q): %v", c.name, err)

			continue
		}

		if len(text) != 1 || text[0] != strconv.FormatFloat(c.want, 'f', -1, 64) {
			t.Errorf("GetStringColumn(%q) = %q, want [%q]", c.name, text, strconv.FormatFloat(c.want, 'f', -1, 64))
		}
	}

	if _, err := table.GetFloatColumn("dec"); !errors.Is(err, fits.ErrColumnNotFound) {
		t.Errorf("GetFloatColumn of a column the table lacks: err = %v, want fits.ErrColumnNotFound", err)
	}

	if _, err := table.GetStringColumn("dec"); !errors.Is(err, fits.ErrColumnNotFound) {
		t.Errorf("GetStringColumn of a column the table lacks: err = %v, want fits.ErrColumnNotFound", err)
	}
}
