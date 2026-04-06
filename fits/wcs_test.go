package fits

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestExtractWCS(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "CRVAL1", Value: "10.0"})
	h.Append(Card{Keyword: "CRVAL2", Value: "20.0"})
	h.Append(Card{Keyword: "CRPIX1", Value: "100.0"})
	h.Append(Card{Keyword: "CRPIX2", Value: "200.0"})
	h.Append(Card{Keyword: "CDELT1", Value: "0.1"})
	h.Append(Card{Keyword: "CDELT2", Value: "0.1"})

	wcs, err := ExtractWCS(h)
	testutil.AssertNoError(t, err)
	if wcs.GetCRVAL()[0] != 10.0 || wcs.GetCRVAL()[1] != 20.0 {
		t.Errorf("expected CRVAL 10.0, 20.0, got %v", wcs.GetCRVAL())
	}

	h2 := NewHeader()
	_, err = ExtractWCS(h2)
	if err == nil {
		t.Errorf("expected error missing NAXIS")
	}

	h2.Append(Card{Keyword: "NAXIS", Value: "0"})
	_, err = ExtractWCS(h2)
	if err == nil {
		t.Errorf("expected error for NAXIS <= 0")
	}
}
