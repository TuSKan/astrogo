package fits_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// TestColumnUnit: a column's unit is its TUNITn value, found by the same name
// matching as the getters, and "" for a column that declares none.
func TestColumnUnit(t *testing.T) {
	t.Parallel()

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
		card("TTYPE1", "'ra      '") + card("TFORM1", "'D       '") + card("TUNIT1", "'h       '") +
		card("TTYPE2", "'DEC     '") + card("TFORM2", "'D       '") + card("TUNIT2", "'deg'") +
		card("TTYPE3", "'MAG     '") + card("TFORM3", "'D       '") +
		"END"

	var payload bytes.Buffer

	_ = binary.Write(&payload, binary.BigEndian, [3]float64{5.92, 7.41, 0.5})

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

	for _, c := range []struct{ column, want string }{
		{"ra", "h"},
		{"RA", "h"}, // matched without case; the value padded to eight characters
		{"dec", "deg"},
		{"MAG", ""}, // declares no unit
	} {
		got, err := table.ColumnUnit(c.column)
		if err != nil {
			t.Errorf("ColumnUnit(%q): %v", c.column, err)

			continue
		}

		if got != c.want {
			t.Errorf("ColumnUnit(%q) = %q, want %q", c.column, got, c.want)
		}
	}

	if _, err := table.ColumnUnit("FLUX"); !errors.Is(err, fits.ErrColumnNotFound) {
		t.Errorf("ColumnUnit of a column the table lacks: err = %v, want fits.ErrColumnNotFound", err)
	}

	if _, err := (&fits.BintableHDU{}).ColumnUnit("RA"); !errors.Is(err, fits.ErrUninitBatch) {
		t.Errorf("ColumnUnit on an unread table: err = %v, want fits.ErrUninitBatch", err)
	}
}
