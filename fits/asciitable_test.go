package fits

import (
	"bytes"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestReadASCIITable(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "TFIELDS", Value: "1"})
	h.Append(Card{Keyword: "NAXIS2", Value: "1"})
	h.Append(Card{Keyword: "NAXIS1", Value: "10"})

	payload := []byte("0123456789")
	padded := make([]byte, 2880)
	copy(padded, payload)

	r := bytes.NewReader(padded)
	hdu, err := ReadASCIITable(h, r)
	testutil.AssertNoError(t, err)

	if hdu.Cols != 1 || hdu.Rows != 1 || hdu.RowSize != 10 {
		t.Errorf("expected 1,1,10 got %d,%d,%d", hdu.Cols, hdu.Rows, hdu.RowSize)
	}

	// Error missing TFIELDS
	h2 := NewHeader()

	_, err = ReadASCIITable(h2, bytes.NewReader(nil))
	if err == nil {
		t.Errorf("expected error missing TFIELDS")
	}
}
