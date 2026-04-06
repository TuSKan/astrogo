package fits

import (
	"bytes"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestReadImage(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "BITPIX", Value: "8"})
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "NAXIS1", Value: "2"})
	h.Append(Card{Keyword: "NAXIS2", Value: "2"})

	payload := []byte{1, 2, 3, 4}

	// Add padding
	padded := make([]byte, 2880)
	copy(padded, payload)

	r := bytes.NewReader(padded)
	img, err := ReadImage(h, r)
	testutil.AssertNoError(t, err)

	if img.Bitpix != 8 {
		t.Errorf("expected 8 bitpix, got %d", img.Bitpix)
	}
	if len(img.Axes) != 2 {
		t.Errorf("expected 2 axes, got %d", len(img.Axes))
	}
	if img.Tensor == nil {
		t.Errorf("expected Tensor, got nil")
	}

	// Test Invalid BITPIX
	h2 := NewHeader()
	// BITPIX parsing error
	_, err = ReadImage(h2, bytes.NewReader(nil))
	if err == nil {
		t.Errorf("expected error for missing BITPIX")
	}

	h2.Append(Card{Keyword: "BITPIX", Value: "99"})
	h2.Append(Card{Keyword: "NAXIS", Value: "1"})
	h2.Append(Card{Keyword: "NAXIS1", Value: "1"})
	_, err = ReadImage(h2, bytes.NewReader(nil))
	if err == nil {
		t.Errorf("expected error for invalid BITPIX value")
	}
}
