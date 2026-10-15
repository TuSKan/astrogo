package fits_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// narrowTable is a BINTABLE holding the column types the scalar getters used
// to read as blanks: a 16-bit integer (I), an unsigned byte (B), a logical
// (L), and a two-element float vector (2E).
func narrowTable(t *testing.T) *fits.BintableHDU {
	t.Helper()

	primary := card("SIMPLE", "T") + card("BITPIX", "8") + card("NAXIS", "0") +
		card("EXTEND", "T") + "END"

	header := card("XTENSION", "'BINTABLE'") +
		card("BITPIX", "8") +
		card("NAXIS", "2") +
		card("NAXIS1", "12") + // 2 + 1 + 1 + 8
		card("NAXIS2", "2") +
		card("PCOUNT", "0") +
		card("GCOUNT", "1") +
		card("TFIELDS", "4") +
		card("TTYPE1", "'SHORT   '") + card("TFORM1", "'I       '") +
		card("TTYPE2", "'BYTE    '") + card("TFORM2", "'B       '") +
		card("TTYPE3", "'FLAG    '") + card("TFORM3", "'L       '") +
		card("TTYPE4", "'VEC     '") + card("TFORM4", "'2E      '") +
		"END"

	var payload bytes.Buffer

	for _, row := range []struct {
		short int16
		b     byte
		flag  byte
		vec   [2]float32
	}{
		{-300, 0, 'T', [2]float32{1, 2}},
		{math.MaxInt16, math.MaxUint8, 'F', [2]float32{3, 4}},
	} {
		_ = binary.Write(&payload, binary.BigEndian, row.short)
		payload.WriteByte(row.b)
		payload.WriteByte(row.flag)
		_ = binary.Write(&payload, binary.BigEndian, row.vec)
	}

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

// TestScalarGettersReadEveryIntegerWidth: a 16-bit integer column and an
// unsigned byte column are ordinary numbers, and both getters read them
// (#769). Neither type was in either getter's switch, so GetFloatColumn gave
// NaN for every row and GetStringColumn a blank, with no error. A logical
// column reads as text the way FITS writes it.
func TestScalarGettersReadEveryIntegerWidth(t *testing.T) {
	t.Parallel()

	table := narrowTable(t)

	for name, want := range map[string][]float64{
		"SHORT": {-300, math.MaxInt16},
		"BYTE":  {0, math.MaxUint8},
	} {
		got, err := table.GetFloatColumn(name)
		if err != nil {
			t.Errorf("GetFloatColumn(%q): %v", name, err)

			continue
		}

		for i := range want {
			if got[i] != want[i] {
				t.Errorf("GetFloatColumn(%q)[%d] = %v, want %v", name, i, got[i], want[i])
			}
		}
	}

	for name, want := range map[string][]string{
		"SHORT": {"-300", "32767"},
		"BYTE":  {"0", "255"},
		"FLAG":  {"T", "F"},
	} {
		got, err := table.GetStringColumn(name)
		if err != nil {
			t.Errorf("GetStringColumn(%q): %v", name, err)

			continue
		}

		for i := range want {
			if got[i] != want[i] {
				t.Errorf("GetStringColumn(%q)[%d] = %q, want %q", name, i, got[i], want[i])
			}
		}
	}
}

// TestScalarGettersRefuseWhatTheyCannotConvert: a logical column has no
// number per row and a vector column has several, so asking for one is
// ErrColumnType rather than a column of NaN or blanks.
func TestScalarGettersRefuseWhatTheyCannotConvert(t *testing.T) {
	t.Parallel()

	table := narrowTable(t)

	if got, err := table.GetFloatColumn("FLAG"); !errors.Is(err, fits.ErrColumnType) {
		t.Errorf("GetFloatColumn(FLAG) = %v, %v; want ErrColumnType", got, err)
	}

	if got, err := table.GetFloatColumn("VEC"); !errors.Is(err, fits.ErrColumnType) {
		t.Errorf("GetFloatColumn(VEC) = %v, %v; want ErrColumnType", got, err)
	}

	if got, err := table.GetStringColumn("VEC"); !errors.Is(err, fits.ErrColumnType) {
		t.Errorf("GetStringColumn(VEC) = %q, %v; want ErrColumnType", got, err)
	}
}
