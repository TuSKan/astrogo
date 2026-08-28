package fits_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

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

// synthetic builds a minimal FITS file: an empty primary HDU followed by a
// BINTABLE with one column of each decoded numeric width and one string
// column. The values are chosen so a byte-order mistake cannot produce them
// by accident.
func synthetic(t *testing.T) []byte {
	t.Helper()

	primary := card("SIMPLE", "T") + card("BITPIX", "8") + card("NAXIS", "0") +
		card("EXTEND", "T") + "END"

	const rowSize = 4 + 8 + 4 + 8 + 6 // E + D + J + K + 6A

	header := card("XTENSION", "'BINTABLE'") +
		card("BITPIX", "8") +
		card("NAXIS", "2") +
		card("NAXIS1", "30") +
		card("NAXIS2", "3") +
		card("PCOUNT", "0") +
		card("GCOUNT", "1") +
		card("TFIELDS", "5") +
		card("TTYPE1", "'FLUX    '") + card("TFORM1", "'E       '") +
		card("TTYPE2", "'WAVELENGTH '") + card("TFORM2", "'D       '") +
		card("TTYPE3", "'COUNT   '") + card("TFORM3", "'J       '") +
		card("TTYPE4", "'BIG     '") + card("TFORM4", "'K       '") +
		card("TTYPE5", "'LABEL   '") + card("TFORM5", "'6A      '") +
		"END"

	if rowSize != 30 {
		t.Fatalf("the fixture row is %d bytes, the header declares 30", rowSize)
	}

	var payload bytes.Buffer

	labels := []string{"alpha", "beta", "gamma"}

	for i := range 3 {
		_ = binary.Write(&payload, binary.BigEndian, float32(1.5+float64(i)))
		_ = binary.Write(&payload, binary.BigEndian, 400.25+float64(i)*100)
		_ = binary.Write(&payload, binary.BigEndian, int32(-7-i))
		_ = binary.Write(&payload, binary.BigEndian, int64(1)<<40+int64(i))

		label := labels[i]
		for len(label) < 6 {
			label += " "
		}

		payload.WriteString(label)
	}

	out := pad([]byte(primary))
	out = append(out, pad([]byte(header))...)
	out = append(out, pad(payload.Bytes())...)

	return out
}

// The decoder must produce the values the file holds, in the right order and
// the right byte order.
//
// Before this existed, ReadBintable built an empty record batch and threw the
// payload away, and Read never dispatched to it in the first place — so every
// caller asking a binary table for a column got no rows and no error. Two of
// them shipped that way.
func TestBintableDecodesValues(t *testing.T) {
	t.Parallel()

	f, err := fits.Read(bytes.NewReader(synthetic(t)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(f.HDUs) != 2 {
		t.Fatalf("got %d HDUs, want 2", len(f.HDUs))
	}

	table, ok := f.HDUs[1].(*fits.BintableHDU)
	if !ok {
		t.Fatalf("HDU 1 is %T, want *fits.BintableHDU", f.HDUs[1])
	}

	if table.Rows != 3 || table.Cols != 5 {
		t.Errorf("table is %dx%d, want 3x5", table.Rows, table.Cols)
	}

	for name, want := range map[string][]float64{
		"FLUX":       {1.5, 2.5, 3.5},
		"WAVELENGTH": {400.25, 500.25, 600.25},
		"COUNT":      {-7, -8, -9},
		"BIG":        {1 << 40, 1<<40 + 1, 1<<40 + 2},
	} {
		got, err := table.GetFloatColumn(name)
		if err != nil {
			t.Errorf("GetFloatColumn(%q): %v", name, err)

			continue
		}

		if len(got) != len(want) {
			t.Errorf("%s has %d rows, want %d", name, len(got), len(want))

			continue
		}

		for i := range want {
			if math.Abs(got[i]-want[i]) > 1e-9 {
				t.Errorf("%s[%d] = %v, want %v", name, i, got[i], want[i])
			}
		}
	}

	labels, err := table.GetStringColumn("LABEL")
	if err != nil {
		t.Fatalf("GetStringColumn: %v", err)
	}

	for i, want := range []string{"alpha", "beta", "gamma"} {
		if labels[i] != want {
			t.Errorf("LABEL[%d] = %q, want %q", i, labels[i], want)
		}
	}
}

// FITS pads a string value out to the width of its card, so a column arrives
// as "WAVELENGTH " rather than "WAVELENGTH". Matching the padded form is what
// made the CALSPEC solar reference look like a file with no columns at all.
func TestBintableTrimsColumnNames(t *testing.T) {
	t.Parallel()

	f, err := fits.Read(bytes.NewReader(synthetic(t)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	table, ok := f.HDUs[1].(*fits.BintableHDU)
	if !ok {
		t.Fatalf("HDU 1 is %T, want *fits.BintableHDU", f.HDUs[1])
	}

	for _, field := range table.Batch.Schema().Fields() {
		if field.Name != strings.TrimSpace(field.Name) {
			t.Errorf("column %q carries FITS padding", field.Name)
		}
	}
}

// A row whose column widths do not add up to NAXIS1 means the layout is being
// read wrong, and every value after the first bad column would be garbage
// drawn from the neighbouring field. That has to fail rather than decode.
func TestBintableRejectsInconsistentRowWidth(t *testing.T) {
	t.Parallel()

	file := synthetic(t)

	// Claim 31 bytes per row against the five columns' actual 30.
	broken := bytes.Replace(file,
		[]byte(card("NAXIS1", "30")), []byte(card("NAXIS1", "31")), 1)

	if bytes.Equal(broken, file) {
		t.Fatal("the fixture did not contain the NAXIS1 card to corrupt")
	}

	if _, err := fits.Read(bytes.NewReader(broken)); err == nil {
		t.Error("a row width disagreeing with the column widths must be rejected")
	}
}
