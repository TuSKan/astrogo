package fits

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// fitsCard renders one 80-byte header card.
func fitsCard(keyword, value string) string {
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

// fitsBlock pads b to the 2880-byte FITS block size.
func fitsBlock(b []byte) []byte {
	if r := len(b) % 2880; r != 0 {
		b = append(b, bytes.Repeat([]byte{' '}, 2880-r)...)
	}

	return b
}

// TestIntegerIDsSurviveLoading: a catalogue whose ID column is 16-bit
// integers (TFORM I) keeps its IDs (#769). GetStringColumn had no case for an
// Int16 column, so every ID loaded as "" and resolving one by ID failed.
func TestIntegerIDsSurviveLoading(t *testing.T) {
	t.Parallel()

	primary := fitsCard("SIMPLE", "T") + fitsCard("BITPIX", "8") + fitsCard("NAXIS", "0") +
		fitsCard("EXTEND", "T") + "END"

	header := fitsCard("XTENSION", "'BINTABLE'") + fitsCard("BITPIX", "8") + fitsCard("NAXIS", "2") +
		fitsCard("NAXIS1", "26") + // 2 + 8 + 8 + 8
		fitsCard("NAXIS2", "2") + fitsCard("PCOUNT", "0") + fitsCard("GCOUNT", "1") +
		fitsCard("TFIELDS", "4") +
		fitsCard("TTYPE1", "'ID      '") + fitsCard("TFORM1", "'I       '") +
		fitsCard("TTYPE2", "'NAME    '") + fitsCard("TFORM2", "'8A      '") +
		fitsCard("TTYPE3", "'RA      '") + fitsCard("TFORM3", "'D       '") +
		fitsCard("TTYPE4", "'DEC     '") + fitsCard("TFORM4", "'D       '") +
		"END"

	var payload bytes.Buffer

	for _, row := range []struct {
		id      int16
		name    string
		ra, dec float64
	}{
		{12, "Betelgse", 88.79, 7.41},
		{345, "Rigel   ", 78.63, -8.20},
	} {
		_ = binary.Write(&payload, binary.BigEndian, row.id)
		payload.WriteString(row.name)
		_ = binary.Write(&payload, binary.BigEndian, row.ra)
		_ = binary.Write(&payload, binary.BigEndian, row.dec)
	}

	data := fitsBlock([]byte(primary))
	data = append(data, fitsBlock([]byte(header))...)
	data = append(data, fitsBlock(payload.Bytes())...)

	path := filepath.Join(t.TempDir(), "int_ids.fits")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, id := range []string{"12", "345"} {
		got, err := p.Resolve(context.Background(), id)
		if err != nil {
			t.Errorf("Resolve(%q): %v", id, err)

			continue
		}

		if got.ID != id {
			t.Errorf("Resolve(%q).ID = %q", id, got.ID)
		}
	}
}
